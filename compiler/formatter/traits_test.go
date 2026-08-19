package formatter

import "testing"

func TestFormatMutatingTraitMethods(t *testing.T) {
	input := `trait Counter {
fn mut set(value:Int)
fn mut()
fn mut mut()
}`
	want := "trait Counter {\n  fn mut set(value: Int)\n  \n  fn mut()\n  \n  fn mut mut()\n}\n"
	got, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	second, err := Format(got, "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != want {
		t.Fatalf("trait formatting is not idempotent:\n%s", second)
	}
}
