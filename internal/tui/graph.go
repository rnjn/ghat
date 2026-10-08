package tui

import (
	"slices"
	"strings"
	"time"

	"github.com/rnjn/ghat/internal/gh"
	"github.com/rnjn/ghat/internal/workflow"
)

// node is one job placed in the dependency graph.
type node struct {
	job      gh.Job
	needs    []int // indexes of the jobs this one depends on
	col      int   // 1 + the longest chain of needs behind it
	critical bool
	// placeholder marks a job the workflow declares that GitHub has not
	// created yet (its needs are still running).
	placeholder bool
}

// graph is a run's jobs with their dependencies, laid out in columns.
type graph struct {
	nodes    []node
	cols     [][]int // node indexes per column, in job order
	critical []int   // the chain of jobs that set the run's length
	inferred bool    // some edges come from timing, not from needs:
	pending  int     // placeholder nodes
}

// buildGraph joins a run's jobs to the workflow's job specs. Jobs that match
// no spec (or every job, when specs is nil) get their edges from timing.
// With pending set (the run is still going), specs no job matches become
// placeholder nodes so the whole pipeline shows before its later stages
// are created.
func buildGraph(jobs []gh.Job, specs []workflow.Job, now time.Time, pending bool) graph {
	g := graph{nodes: make([]node, len(jobs))}
	byKey := map[string][]int{}
	spec := make([]int, len(jobs))
	for i, j := range jobs {
		g.nodes[i].job = j
		spec[i] = matchSpec(j.Name, specs)
		if spec[i] >= 0 {
			byKey[specs[spec[i]].Key] = append(byKey[specs[spec[i]].Key], i)
		}
	}
	if pending {
		for si, sp := range specs {
			if len(byKey[sp.Key]) > 0 {
				continue
			}
			name := sp.Name
			if name == "" || strings.Contains(name, "${{") {
				name = sp.Key
			}
			g.nodes = append(g.nodes, node{job: gh.Job{Name: name, Status: "pending"}, placeholder: true})
			spec = append(spec, si)
			byKey[sp.Key] = []int{len(g.nodes) - 1}
			g.pending++
		}
	}
	for i := range g.nodes {
		if spec[i] < 0 {
			g.nodes[i].needs = inferNeeds(jobs, i)
			g.inferred = true
			continue
		}
		for _, k := range specs[spec[i]].Needs {
			g.nodes[i].needs = append(g.nodes[i].needs, byKey[k]...)
		}
	}
	g.layout()
	g.findCritical(now)
	return g
}

// matchSpec finds the spec a job name was generated from: the name minus a
// matrix suffix " (…)" and a reusable-workflow suffix " / …" equals the
// spec's name or key. It returns -1 for no match.
func matchSpec(name string, specs []workflow.Job) int {
	base := name
	if i := strings.Index(base, " / "); i >= 0 {
		base = base[:i]
	}
	if i := strings.LastIndex(base, " ("); i >= 0 && strings.HasSuffix(base, ")") {
		base = base[:i]
	}
	for i, s := range specs {
		if s.Name != "" && s.Name == base {
			return i
		}
	}
	for i, s := range specs {
		if s.Key == base {
			return i
		}
	}
	return -1
}

// inferNeeds guesses job i's dependencies from timing: the jobs that
// finished before it started (every started job, for one still queued),
// minus those another such job already waited for.
func inferNeeds(jobs []gh.Job, i int) []int {
	v := jobs[i]
	var before []int
	for k, u := range jobs {
		if k == i || u.StartedAt.IsZero() {
			continue
		}
		if v.StartedAt.IsZero() || (u.Status == "completed" && !u.CompletedAt.After(v.StartedAt)) {
			before = append(before, k)
		}
	}
	var out []int
	for _, u := range before {
		covered := false
		for _, w := range before {
			if w != u && jobs[u].Status == "completed" && !jobs[u].CompletedAt.After(jobs[w].StartedAt) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, u)
		}
	}
	return out
}

// layout assigns columns by longest chain of needs (a cycle counts as a
// root) and lists the nodes of each column.
func (g *graph) layout() {
	state := make([]int8, len(g.nodes)) // 0 new, 1 visiting, 2 done
	var depth func(i int) int
	depth = func(i int) int {
		switch state[i] {
		case 2:
			return g.nodes[i].col
		case 1:
			return -1
		}
		state[i] = 1
		d := 0
		for _, u := range g.nodes[i].needs {
			d = max(d, depth(u)+1)
		}
		g.nodes[i].col, state[i] = d, 2
		return d
	}
	used := map[int]bool{}
	for i := range g.nodes {
		used[depth(i)] = true
	}
	order := make([]int, 0, len(used))
	for c := range used {
		order = append(order, c)
	}
	slices.Sort(order)
	rank := map[int]int{}
	for r, c := range order {
		rank[c] = r
	}
	g.cols = make([][]int, len(order))
	for i := range g.nodes {
		c := rank[g.nodes[i].col]
		g.nodes[i].col = c
		g.cols[c] = append(g.cols[c], i)
	}
}

// end is when a job stopped, now for one still running, zero if unstarted.
func jobEnd(j gh.Job, now time.Time) time.Time {
	switch {
	case j.Status == "completed":
		return j.CompletedAt
	case !j.StartedAt.IsZero():
		return now
	}
	return time.Time{}
}

// findCritical walks back from the last job to finish through the
// dependency that finished last each time.
func (g *graph) findCritical(now time.Time) {
	last := -1
	for i, n := range g.nodes {
		if e := jobEnd(n.job, now); !e.IsZero() && (last < 0 || e.After(jobEnd(g.nodes[last].job, now))) {
			last = i
		}
	}
	if last < 0 {
		return
	}
	seen := map[int]bool{}
	var path []int
	for cur := last; cur >= 0 && !seen[cur]; {
		seen[cur] = true
		path = append(path, cur)
		g.nodes[cur].critical = true
		next := -1
		for _, u := range g.nodes[cur].needs {
			if next < 0 || jobEnd(g.nodes[u].job, now).After(jobEnd(g.nodes[next].job, now)) {
				next = u
			}
		}
		cur = next
	}
	slices.Reverse(path)
	g.critical = path
}
