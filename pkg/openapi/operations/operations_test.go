package operations

import "testing"

func TestTrimString(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{
			input:  "\n\n1\n2\n\n\n",
			output: "1\n2",
		},
		{
			input:  "    1\n2    ",
			output: "1\n2",
		},
		{
			input:  "\n    1\n2    \n ",
			output: "1\n2",
		},
	}

	for _, test := range tests {
		if output := trimString(test.input); output != test.output {
			t.Errorf("trimString incorrect output\nexpected: %s\ngot: %s", test.output, output)
		}
	}
}
