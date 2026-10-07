package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"ghtui/internal/actions"
	"ghtui/internal/workflow"
)

type fieldKind int

const (
	textField fieldKind = iota
	boolField
	choiceField
)

// field is one row of the form: the ref, then one per workflow input.
type field struct {
	label   string
	hint    string
	kind    fieldKind
	input   textinput.Model // text fields
	value   string          // bool and choice fields
	options []string
}

func (f *field) val() string {
	if f.kind == textField {
		return f.input.Value()
	}
	return f.value
}

// dispatchForm collects the ref and inputs for one workflow.
type dispatchForm struct {
	repoKey string
	ref     string
	wf      actions.Dispatchable
	fields  []field
	focus   int
	err     string
}

func newDispatchForm(repoKey, ref string, wf actions.Dispatchable) *dispatchForm {
	f := &dispatchForm{repoKey: repoKey, ref: ref, wf: wf}
	f.fields = append(f.fields, newText("ref", "branch or tag to run on", ref))
	for _, in := range wf.Inputs {
		f.fields = append(f.fields, inputField(in))
	}
	f.setFocus(0)
	return f
}

func newText(label, hint, value string) field {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetValue(value)
	return field{label: label, hint: hint, kind: textField, input: ti}
}

func inputField(in workflow.Input) field {
	hint := in.Type
	if in.Required {
		hint += ", required"
	}
	if in.Description != "" {
		hint += " · " + in.Description
	}
	switch in.Type {
	case "boolean":
		v := in.Default
		if v != "true" {
			v = "false"
		}
		return field{label: in.Name, hint: hint, kind: boolField, value: v}
	case "choice":
		v := in.Default
		if v == "" && len(in.Options) > 0 {
			v = in.Options[0]
		}
		return field{label: in.Name, hint: hint, kind: choiceField, value: v, options: in.Options}
	}
	return newText(in.Name, hint, in.Default)
}

func (f *dispatchForm) Title() string { return "Dispatch " + f.wf.Workflow.Name }

// CapturesKeys makes the model pass typed keys to the form.
func (f *dispatchForm) CapturesKeys() bool { return true }

func (f *dispatchForm) setFocus(i int) {
	f.focus = max(0, min(i, len(f.fields)-1))
	for j := range f.fields {
		if f.fields[j].kind != textField {
			continue
		}
		if j == f.focus {
			f.fields[j].input.Focus()
		} else {
			f.fields[j].input.Blur()
		}
	}
}

func (f *dispatchForm) Update(msg tea.Msg, ctx *Context) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	cur := &f.fields[f.focus]
	switch k.String() {
	case "tab", "down":
		f.setFocus(f.focus + 1)
		return f, nil
	case "shift+tab", "up":
		f.setFocus(f.focus - 1)
		return f, nil
	case "enter":
		if f.focus < len(f.fields)-1 {
			f.setFocus(f.focus + 1)
			return f, nil
		}
		return f, f.submit(ctx)
	case "space", "left", "right":
		switch cur.kind {
		case boolField:
			cur.value = map[string]string{"true": "false", "false": "true"}[cur.value]
			return f, nil
		case choiceField:
			cur.value = cycle(cur.options, cur.value, k.String() == "left")
			return f, nil
		}
	}
	if cur.kind == textField {
		var cmd tea.Cmd
		cur.input, cmd = cur.input.Update(msg)
		return f, cmd
	}
	return f, nil
}

func cycle(options []string, v string, back bool) string {
	if len(options) == 0 {
		return v
	}
	i := slices.Index(options, v)
	switch {
	case i < 0:
		return options[0]
	case back:
		return options[(i-1+len(options))%len(options)]
	}
	return options[(i+1)%len(options)]
}

// submit validates and asks for confirmation.
func (f *dispatchForm) submit(ctx *Context) tea.Cmd {
	ref := strings.TrimSpace(f.fields[0].val())
	values := map[string]string{}
	for i, in := range f.wf.Inputs {
		values[in.Name] = f.fields[i+1].val()
	}
	if ref == "" {
		f.err = "ref is required"
		return nil
	}
	if err := actions.Validate(f.wf.Inputs, values); err != nil {
		f.err = err.Error()
		return nil
	}
	f.err = ""
	acts, refresh := ctx.Actions, ctx.Refresh
	owner, repo := splitKey(f.repoKey)
	wf, inputs, repoKey := f.wf.Workflow, f.wf.Inputs, f.repoKey
	return ask(fmt.Sprintf("Dispatch %s on %s?", wf.Name, ref), func() tea.Msg {
		text, err := acts.Dispatch(context.Background(), owner, repo, wf, ref, inputs, values)
		if err != nil {
			return ActionResult{Err: err}
		}
		refresh("runs:" + repoKey)
		return ActionResult{Text: text, From: f}
	})
}

func (f *dispatchForm) View(ctx *Context, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	labelW := 0
	for _, fl := range f.fields {
		labelW = max(labelW, len(fl.label))
	}
	var rows []string
	for i, fl := range f.fields {
		marker := "  "
		if i == f.focus {
			marker = "› "
		}
		var v string
		switch fl.kind {
		case textField:
			v = fl.input.View()
		case boolField:
			v = fl.value + styleDim.Render("  (space toggles)")
		case choiceField:
			v = "◂ " + fl.value + " ▸" + styleDim.Render("  ("+strings.Join(fl.options, ", ")+")")
		}
		rows = append(rows, fmt.Sprintf("%s%-*s  %s", marker, labelW, fl.label, v), "    "+styleDim.Render(fl.hint))
	}
	foot := []string{""}
	if f.err != "" {
		foot = append(foot, "  "+styleError.Render(f.err))
	}
	foot = append(foot, styleDim.Render("  tab/↓ next · shift+tab/↑ back · enter on last field submits · esc cancels"))
	avail := max(2, height-len(foot)-1)
	start := 0
	if focusRow := f.focus * 2; focusRow+2 > avail {
		start = focusRow + 2 - avail
	}
	end := min(len(rows), start+avail)
	head := styleHeader.Render(fmt.Sprintf("  %s · %s", f.wf.Workflow.Name, f.repoKey))
	out := append([]string{head}, rows[start:end]...)
	return fit(append(out, foot...), width, height)
}
