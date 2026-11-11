package plan

import (
	"reflect"
	"testing"
)

func okInput(goos string) Input {
	return Input{
		GOOS:        goos,
		ClusterNoun: "the course cluster",
		Tools: []Tool{
			{ID: "kubectl", Name: "kubectl", Version: "1.36.6", DownloadBytes: 61115904},
			{ID: "kubelogin", Name: "kubelogin", Version: "1.36.4", DownloadBytes: 11471605},
		},
		PathOK:     true,
		RCFile:     "~/.zshrc",
		Kubeconfig: KubeconfigInput{EntriesOK: true, CurrentOK: true, ModeOK: true},
	}
}

func TestCompute(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		change func(*Input)
		want   []string
	}{
		{name: "everything set up", goos: "windows", change: func(*Input) {}, want: nil},
		{name: "clean machine", goos: "windows", change: func(in *Input) {
			in.Tools[0].Status, in.Tools[1].Status = ToolMissing, ToolMissing
			in.SelfNeedsCopy, in.PathOK = true, false
			in.Kubeconfig = KubeconfigInput{ModeOK: true}
		}, want: []string{
			"Install kubectl 1.36.6",
			"Install kubelogin 1.36.4",
			"Copy kube-onramp into its own folder",
			"Add kube-onramp's folder to the start of your PATH (the list of folders where Windows looks for programs)",
			"Add the course cluster to your Kubernetes settings (a backup is made first)",
		}},
		{name: "machine B: course config, mode 644", goos: "darwin", change: func(in *Input) {
			in.Tools[0].Status, in.Tools[1].Status = ToolMissing, ToolMissing
			in.PathOK = false
			in.Kubeconfig = KubeconfigInput{ModeOK: false}
		}, want: []string{
			"Install kubectl 1.36.6",
			"Install kubelogin 1.36.4",
			"Add kube-onramp's folder to your PATH in ~/.zshrc (the list of folders where the terminal looks for programs)",
			"Add the course cluster to your Kubernetes settings (a backup is made first)",
			"Make your Kubernetes settings file private (only you can read it)",
		}},
		{name: "outdated and broken tools", goos: "windows", change: func(in *Input) {
			in.Tools[0].Status, in.Tools[1].Status = ToolOutdated, ToolBroken
		}, want: []string{"Update kubectl to 1.36.6", "Repair kubelogin 1.36.4 (download it again)"}},
		{name: "entries changed", goos: "windows", change: func(in *Input) {
			in.Kubeconfig = KubeconfigInput{AnyPresent: true, CurrentOK: true, ModeOK: true}
		}, want: []string{"Update the course cluster in your Kubernetes settings (a backup is made first)"}},
		{name: "only the current context", goos: "windows", change: func(in *Input) {
			in.Kubeconfig.CurrentOK = false
		}, want: []string{"Make the course cluster the default in your Kubernetes settings (a backup is made first)"}},
		{name: "broken file: no separate mode fix", goos: "darwin", change: func(in *Input) {
			in.Kubeconfig = KubeconfigInput{Broken: true}
		}, want: []string{"Replace your Kubernetes settings file, because it can't be read (a backup is made first)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := okInput(tt.goos)
			tt.change(&in)
			var got []string
			for _, a := range Compute(in) {
				got = append(got, a.Text)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Compute()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestDownloadBytes(t *testing.T) {
	in := okInput("windows")
	in.Tools[0].Status = ToolMissing
	actions := Compute(in)
	if got := DownloadBytes(actions); got != 61115904 {
		t.Errorf("DownloadBytes = %d", got)
	}
	if got := SizeText(DownloadBytes(actions)); got != "about 58 MB" {
		t.Errorf("SizeText = %q", got)
	}
	if !Has(actions, InstallTool) || Has(actions, UpdatePath) {
		t.Error("Has is wrong")
	}
}
