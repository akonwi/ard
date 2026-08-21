package gotarget

import "testing"

func TestGenericCompatibilityPreservesGoRepresentations(t *testing.T) {
	program := lowerParitySource(t, `
		trait Widget {
			fn render() Int
		}

		struct Root {}
		impl Widget for Root {
			fn render() Int { 1 }
		}

		struct Holder<$T> {
			value: $T,
		}

		struct Marker<$T> {
			value: Int,
		}

		struct OptionalHolder<$T> {
			value: $T?,
		}

		struct OptionalWidgetRef {
			value: (mut Widget)?,
		}

		struct WidgetListRef<$T> {
			values: mut [$T],
		}

		fn marker(seed: $T) Marker<$T> {
			Marker{value: 1}
		}

		fn main() Int {
			let holder = Holder<Widget>{value: Root{}}
			let optional = OptionalHolder<Widget>{value: Root{}}
			let root = mut Root{}
			let optional_ref = OptionalWidgetRef{value: root}
			let widget_values: [Widget] = [Root{}]
			let widget_list_ref = WidgetListRef<Widget>{values: mut widget_values}
			let marked = marker("context")
			let direct_value = holder.value.render()
			let optional_value = optional.value.expect("widget").render()
			let optional_ref_value = optional_ref.value.expect("widget reference").render()
			direct_value + optional_value + optional_ref_value + marked.value
		}
	`)
	if got := runGoTargetParityJSON(t, program); got != "4" {
		t.Fatalf("result = %s, want 4", got)
	}
}
