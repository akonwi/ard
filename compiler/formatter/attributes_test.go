package formatter

import "testing"

func TestFormatStructFieldAttributes(t *testing.T) {
	input := `struct User { #json( name:"displayName",omit:none ) display_name:Str?, #json(skip:true) password_hash:Str }`
	want := `struct User {
  #json(name: "displayName", omit: none)
  display_name: Str?,
  #json(skip: true)
  password_hash: Str,
}
`
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
		t.Fatalf("attribute formatting is not idempotent:\n%s", second)
	}
}

func TestFormatCommentBetweenAttributeAndField(t *testing.T) {
	input := `struct User {
  #json(name: "displayName")
  // Public API spelling.
  display_name: Str,
  age: Int,
}`
	want := `struct User {
  #json(name: "displayName")
  // Public API spelling.
  display_name: Str,
  age: Int,
}
`
	got, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestFormatLongStructFieldAttribute(t *testing.T) {
	input := `struct User {
  #json(name: "a_very_long_external_name_that_pushes_the_attribute_past_the_canonical_formatter_line_width_for_this_field")
  name: Str,
}`
	want := `struct User {
  #json(
    name: "a_very_long_external_name_that_pushes_the_attribute_past_the_canonical_formatter_line_width_for_this_field",
  )
  name: Str,
}
`
	got, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
