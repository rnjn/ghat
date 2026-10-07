package gh

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultBranchDecoded(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"api","owner":{"login":"acme"},"default_branch":"trunk"}`))
	})
	r, err := c.GetRepo(context.Background(), "acme", "api")
	if err != nil || r.DefaultBranch != "trunk" {
		t.Fatalf("repo %+v err %v", r, err)
	}
}

func TestListWorkflowsPaginates(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/actions/workflows" {
			t.Errorf("path %q", r.URL.Path)
		}
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/r/actions/workflows?per_page=100&page=2>; rel="next"`, srv.URL))
			_, _ = w.Write([]byte(`{"total_count":2,"workflows":[{"id":1,"name":"CI","path":".github/workflows/ci.yml","state":"active"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"total_count":2,"workflows":[{"id":2,"name":"Deploy","path":".github/workflows/deploy.yml","state":"disabled_manually"}]}`))
	}))
	defer srv.Close()
	wfs, err := New("tok", WithBaseURL(srv.URL)).ListWorkflows(context.Background(), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(wfs) != 2 || wfs[0].Name != "CI" || wfs[1].ID != 2 || wfs[1].State != "disabled_manually" || wfs[0].Path != ".github/workflows/ci.yml" {
		t.Fatalf("workflows = %+v", wfs)
	}
}

func TestWorkflowFile(t *testing.T) {
	content := "name: CI\non:\n  workflow_dispatch:\n"
	enc := base64.StdEncoding.EncodeToString([]byte(content))
	enc = enc[:10] + "\n" + enc[10:] // GitHub wraps base64 at 60 columns
	var gotRef, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotRef = r.URL.Path, r.URL.Query().Get("ref")
		if strings.HasSuffix(r.URL.Path, "missing.yml") {
			w.WriteHeader(404)
			return
		}
		_, _ = fmt.Fprintf(w, `{"encoding":"base64","content":%q}`, enc)
	})
	b, err := c.WorkflowFile(context.Background(), "o", "r", ".github/workflows/ci.yml", "feat/x")
	if err != nil || string(b) != content {
		t.Fatalf("content %q err %v", b, err)
	}
	if gotPath != "/repos/o/r/contents/.github/workflows/ci.yml" || gotRef != "feat/x" {
		t.Fatalf("path %q ref %q", gotPath, gotRef)
	}
	if _, err := c.WorkflowFile(context.Background(), "o", "r", ".github/workflows/ci.yml", ""); err != nil || gotRef != "" {
		t.Fatalf("empty ref sent %q (err %v)", gotRef, err)
	}
	if _, err := c.WorkflowFile(context.Background(), "o", "r", "missing.yml", ""); !IsNotFound(err) {
		t.Fatalf("err = %v", err)
	}
}
