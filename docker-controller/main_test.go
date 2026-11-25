package main

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

func TestDecodeSeedConfig(t *testing.T) {
	if got, err := decodeSeedConfig(""); err != nil || got != nil {
		t.Fatalf("empty config must be a no-op, got %v, %v", got, err)
	}
	valid := base64.StdEncoding.EncodeToString([]byte(`{"default_provider":"openai"}`))
	got, err := decodeSeedConfig(valid)
	if err != nil || string(got) != `{"default_provider":"openai"}` {
		t.Fatalf("valid config rejected: %v, %v", got, err)
	}
	for name, in := range map[string]string{
		"not base64": "!!!",
		"not json":   base64.StdEncoding.EncodeToString([]byte("hello")),
		"json array": base64.StdEncoding.EncodeToString([]byte("[1,2]")),
	} {
		if _, err := decodeSeedConfig(in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestConfigTar(t *testing.T) {
	content := []byte("{\"a\":1}\n")
	archive, err := configTar(content, "config.json")
	if err != nil {
		t.Fatalf("configTar: %v", err)
	}
	names, got := readTar(t, archive)
	if len(names) != 1 || names[0] != "config.json" || string(got) != string(content) {
		t.Fatalf("unexpected tar: %v %q", names, got)
	}
}

func TestConfigTarCreatesParentFolders(t *testing.T) {
	archive, err := configTar([]byte("{}"), "custom/nested/my-config.json")
	if err != nil {
		t.Fatalf("configTar: %v", err)
	}
	names, got := readTar(t, archive)
	want := []string{"custom/", "custom/nested/", "custom/nested/my-config.json"}
	if len(names) != len(want) {
		t.Fatalf("entries %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("entries %v, want %v", names, want)
		}
	}
	if string(got) != "{}" {
		t.Fatalf("content %q", got)
	}
}

// readTar returns the entry names and the content of the last (file) entry.
func readTar(t *testing.T, archive []byte) ([]string, []byte) {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(archive))
	var names []string
	var last []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return names, last
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		names = append(names, hdr.Name)
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		last, err = io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
	}
}

func TestSeedPathFollowsTemplateConfigPath(t *testing.T) {
	cases := []struct {
		env  []string
		want string
	}{
		{env: []string{"PATH=/bin"}, want: "config.json"},
		{env: []string{"CONFIG_PATH=/data/config.json"}, want: "config.json"},
		{env: []string{"CONFIG_PATH=/data/custom/my-config.json"}, want: "custom/my-config.json"},
		// Outside the private volume (or escaping it): keep the file in /data.
		{env: []string{"CONFIG_PATH=/etc/config.json"}, want: "config.json"},
		{env: []string{"CONFIG_PATH=/data/../etc/config.json"}, want: "config.json"},
	}
	for _, tc := range cases {
		if got := seedRelativePath(tc.env); got != tc.want {
			t.Errorf("seedRelativePath(%v) = %s; want %s", tc.env, got, tc.want)
		}
	}
}

func TestTemplateEnvDropsControllerCredentials(t *testing.T) {
	env := []string{"PATH=/bin", "CONTROLLER_TOKEN=secret", "CONTROLLER_URL=http://x", "API_KEY=k"}
	got := templateEnv(env)
	if strings.Join(got, "|") != "PATH=/bin|API_KEY=k" {
		t.Fatalf("controller credentials leaked: %v", got)
	}
}

func TestChooseImagePrefersRunningImage(t *testing.T) {
	src := &containerInspect{
		Image:  "sha256:old",
		Config: containerConfig{Image: "useless-agent-backend"},
	}

	got, err := chooseImage(src, func(ref string) bool { return true })
	if err != nil || got != "sha256:old" {
		t.Fatalf("chooseImage = %q, %v; want the image the template runs on", got, err)
	}
}

// A rebuild can remove the image ID the running backend was created from
// while the container keeps running. The copy must then use the image
// reference (tag) instead of failing with "No such image".
func TestChooseImageFallsBackToReference(t *testing.T) {
	src := &containerInspect{
		Image:  "sha256:old",
		Config: containerConfig{Image: "useless-agent-backend"},
	}

	got, err := chooseImage(src, func(ref string) bool {
		return ref == "useless-agent-backend"
	})
	if err != nil || got != "useless-agent-backend" {
		t.Fatalf("chooseImage = %q, %v; want the rebuilt image reference", got, err)
	}
}

func TestChooseImageReportsMissingImage(t *testing.T) {
	src := &containerInspect{Image: "sha256:old"}
	if _, err := chooseImage(src, func(string) bool { return false }); err == nil {
		t.Fatal("expected an error when no image is available")
	}
}
