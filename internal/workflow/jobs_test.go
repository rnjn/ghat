package workflow

import (
	"reflect"
	"testing"
)

func TestParseJobs(t *testing.T) {
	file := []byte(`
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
  lint:
    name: Lint ${{ matrix.os }}
    needs: build
  test:
    needs: [build, lint]
  deploy:
    needs:
      - test
`)
	got, err := ParseJobs(file)
	if err != nil {
		t.Fatal(err)
	}
	want := []Job{
		{Key: "build"},
		{Key: "lint", Name: "Lint ${{ matrix.os }}", Needs: []string{"build"}},
		{Key: "test", Needs: []string{"build", "lint"}},
		{Key: "deploy", Needs: []string{"test"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseJobsNoJobsAndBadYAML(t *testing.T) {
	if got, err := ParseJobs([]byte("on: push\n")); err != nil || len(got) != 0 {
		t.Fatalf("no jobs: %v %v", got, err)
	}
	if _, err := ParseJobs([]byte("jobs: [")); err == nil {
		t.Fatal("bad yaml accepted")
	}
	if _, err := ParseJobs([]byte("jobs:\n  a:\n    needs: {x: 1}\n")); err == nil {
		t.Fatal("mapping needs accepted")
	}
}
