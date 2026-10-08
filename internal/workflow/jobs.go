package workflow

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Job is one entry of a workflow's jobs map. Name is the job's name: field
// verbatim (it may hold an expression), or "" when it has none. Needs lists
// the keys of the jobs it depends on.
type Job struct {
	Key   string
	Name  string
	Needs []string
}

// ParseJobs returns a workflow's jobs in file order.
func ParseJobs(file []byte) ([]Job, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(file, &doc); err != nil {
		return nil, fmt.Errorf("parse workflow: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	jobs := mapValue(doc.Content[0], "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		return nil, nil
	}
	var out []Job
	for i := 0; i+1 < len(jobs.Content); i += 2 {
		j := Job{Key: jobs.Content[i].Value}
		spec := jobs.Content[i+1]
		if v := mapValue(spec, "name"); v != nil {
			j.Name = v.Value
		}
		switch needs := mapValue(spec, "needs"); {
		case needs == nil:
		case needs.Kind == yaml.ScalarNode:
			j.Needs = []string{needs.Value}
		case needs.Kind == yaml.SequenceNode:
			if err := needs.Decode(&j.Needs); err != nil {
				return nil, fmt.Errorf("job %s: needs: %w", j.Key, err)
			}
		default:
			return nil, fmt.Errorf("job %s: needs must be a job key or a list", j.Key)
		}
		out = append(out, j)
	}
	return out, nil
}
