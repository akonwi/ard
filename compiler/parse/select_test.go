package parse

import "testing"

func TestSelectExpression(t *testing.T) {
	runTests(t, []test{
		{
			name: "all arm forms",
			input: `
				select {
					let job = jobs.recv() => run(job),
					results.send(value) => done(),
					timeout.recv() => giveUp(),
					_ => idle(),
				}`,
			output: Program{
				Imports: []Import{},
				Statements: []Statement{
					&SelectExpression{
						Cases: []SelectCase{
							{
								Binding: &Identifier{Name: "job"},
								Op: &InstanceMethod{
									Target: &Identifier{Name: "jobs"},
									Method: FunctionCall{
										Name:     "recv",
										Args:     []Argument{},
										Comments: []Comment{},
									},
								},
								Body: []Statement{
									&FunctionCall{
										Name:     "run",
										Args:     []Argument{{Name: "", Value: &Identifier{Name: "job"}}},
										Comments: []Comment{},
									},
								},
							},
							{
								Op: &InstanceMethod{
									Target: &Identifier{Name: "results"},
									Method: FunctionCall{
										Name:     "send",
										Args:     []Argument{{Name: "", Value: &Identifier{Name: "value"}}},
										Comments: []Comment{},
									},
								},
								Body: []Statement{
									&FunctionCall{
										Name:     "done",
										Args:     []Argument{},
										Comments: []Comment{},
									},
								},
							},
							{
								Op: &InstanceMethod{
									Target: &Identifier{Name: "timeout"},
									Method: FunctionCall{
										Name:     "recv",
										Args:     []Argument{},
										Comments: []Comment{},
									},
								},
								Body: []Statement{
									&FunctionCall{
										Name:     "giveUp",
										Args:     []Argument{},
										Comments: []Comment{},
									},
								},
							},
							{
								Op: &Identifier{Name: "_"},
								Body: []Statement{
									&FunctionCall{
										Name:     "idle",
										Args:     []Argument{},
										Comments: []Comment{},
									},
								},
							},
						},
					},
				},
			},
		},
	})
}
