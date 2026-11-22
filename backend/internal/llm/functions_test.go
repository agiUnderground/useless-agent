package llm

import (
	"reflect"
	"testing"
)

func TestParseSubtasksJSONRawArray(t *testing.T) {
	got, err := parseSubtasksJSON(`[{"id":1,"description":"open menu"},{"id":2,"description":"click browser"}]`)
	if err != nil {
		t.Fatal(err)
	}
	want := []SubTask{{1, "open menu"}, {2, "click browser"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestParseSubtasksJSONFencedArray(t *testing.T) {
	content := "Here is the plan:\n```json\n[{\"id\":1,\"description\":\"a\"},{\"id\":2,\"description\":\"b\"}]\n```\nDone."
	got, err := parseSubtasksJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Description != "a" || got[1].Description != "b" {
		t.Fatalf("got %#v", got)
	}
}

func TestParseSubtasksJSONWrappedObject(t *testing.T) {
	for _, key := range []string{"subtasks", "steps", "tasks"} {
		content := `{"` + key + `":[{"id":7,"description":"wrapped"}]}`
		got, err := parseSubtasksJSON(content)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Id != 7 || got[0].Description != "wrapped" {
			t.Fatalf("key %s: got %#v", key, got)
		}
	}
}

func TestParseSubtasksJSONSingleObject(t *testing.T) {
	got, err := parseSubtasksJSON(`{"id":3,"description":"single"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (SubTask{3, "single"}) {
		t.Fatalf("got %#v", got)
	}
}

func TestParseSubtasksJSONBracketsAndBracesInStrings(t *testing.T) {
	content := `[{"id":1,"description":"click [Start] {menu}"},{"id":0,"description":"  "},{"description":"auto id"}]`
	got, err := parseSubtasksJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	want := []SubTask{{1, "click [Start] {menu}"}, {2, "auto id"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestParseSubtasksJSONInvalid(t *testing.T) {
	if _, err := parseSubtasksJSON("not json at all"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseVerdictJSONRawObject(t *testing.T) {
	v, err := parseVerdictJSON(`{"isGoalAchieved":true,"description":"done","newPrompt":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.IsGoalAchieved || v.Description != "done" {
		t.Fatalf("got %#v", v)
	}
}

func TestParseVerdictJSONFencedObject(t *testing.T) {
	content := "```\n{\"isGoalAchieved\":false,\"description\":\"partial {not} done [yet]\",\"newPrompt\":\"continue\"}\n```"
	v, err := parseVerdictJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	if v.IsGoalAchieved || v.NewPrompt != "continue" {
		t.Fatalf("got %#v", v)
	}
}

func TestParseVerdictJSONInvalid(t *testing.T) {
	if _, err := parseVerdictJSON("no object here"); err == nil {
		t.Fatal("expected error")
	}
}
