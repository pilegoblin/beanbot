package gemini

import "testing"

func TestAFencedDocumentIsUnwrapped(t *testing.T) {
	// The instruction forbids code fences and the model supplies them anyway
	// often enough that "```markdown" would become the first line of a Memory.
	got := stripCodeFence("```markdown\n## People\n- Steve likes boats.\n```")

	want := "## People\n- Steve likes boats."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAnUnfencedDocumentIsUntouched(t *testing.T) {
	doc := "## People\n- Steve likes boats.\n"

	if got := stripCodeFence(doc); got != doc {
		t.Errorf("got %q, want %q", got, doc)
	}
}

func TestFencedCodeInsideTheDocumentSurvives(t *testing.T) {
	got := stripCodeFence("```\n## Jokes\n- Drew's build script:\n```sh\nmake it\n```\n```")

	if got == "" {
		t.Fatal("the document was emptied")
	}
	if want := "## Jokes"; got[:len(want)] != want {
		t.Errorf("the outer fence was not stripped cleanly: %q", got)
	}
}
