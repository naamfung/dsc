package main

import "testing"

func TestFormatShellResult(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		exitCode int32
		want     string
	}{
		{
			name:     "非零退出碼且有輸出，追加 [exit_code: N]",
			output:   "error: boom",
			exitCode: 1,
			want:     "error: boom\n[exit_code: 1]\n",
		},
		{
			name:     "非零退出碼且無輸出，也追加 [exit_code: N]",
			output:   "",
			exitCode: 2,
			want:     "\n[exit_code: 2]\n",
		},
		{
			name:     "零退出碼且有輸出，不附加 [exit_code: 0]",
			output:   "hello world",
			exitCode: 0,
			want:     "hello world",
		},
		{
			name:     "零退出碼且全空白輸出，返回 [exit_code: 0]",
			output:   "  \n\t ",
			exitCode: 0,
			want:     "\n[exit_code: 0]\n",
		},
		{
			name:     "零退出碼且零輸出，返回 [exit_code: 0]",
			output:   "",
			exitCode: 0,
			want:     "\n[exit_code: 0]\n",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatShellResult(c.output, c.exitCode); got != c.want {
				t.Errorf("formatShellResult(%q, %d) = %q, want %q", c.output, c.exitCode, got, c.want)
			}
		})
	}
}
