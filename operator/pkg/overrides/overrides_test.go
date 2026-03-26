package overrides

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func newTestRunner(g *WithT, root string, o Overrides) *Runner {
	r, err := NewRunner(
		filepath.Join(root, "operator", "upstream-kustomizations"),
		filepath.Join(root, "operator", "pkg", "manifests"),
		filepath.Join(root, ".tmp"),
		o,
	)
	g.Expect(err).ToNot(HaveOccurred())
	return r
}

func TestParseAndValidateFromYAML(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	_, err := ParseAndValidateFromYAML(`
- name: segment-bridge
  git:
    - sourceRepo: konflux-ci/segment-bridge
      remote:
        repo: https://github.com/konflux-ci/segment-bridge
        ref: abc123
  images:
    - orig: quay.io/konflux-ci/segment-bridge
      replacement: quay.io/example/segment-bridge:pr
`)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestParseAndValidateFromYAMLRejectsInvalidRule(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	_, err := ParseAndValidateFromYAML(`
- name: segment-bridge
  git:
    - sourceRepo: konflux-ci/segment-bridge
  images: []
`)
	g.Expect(err).To(HaveOccurred())
}

func TestApplyImageOverridesInManifests(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	manifestDir := filepath.Join(root, "operator", "pkg", "manifests", "segment-bridge")
	g.Expect(os.MkdirAll(manifestDir, 0o755)).To(Succeed())
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: segment-bridge
spec:
  template:
    spec:
      containers:
        - name: app
          image: quay.io/konflux-ci/segment-bridge:old
`
	path := filepath.Join(manifestDir, "manifests.yaml")
	g.Expect(os.WriteFile(path, []byte(manifest), 0o644)).To(Succeed())

	o := Overrides{
		{
			Name: "segment-bridge",
			Images: []ImageOverride{
				{Orig: "quay.io/konflux-ci/segment-bridge", Replacement: "quay.io/example/segment-bridge:new"},
			},
		},
	}
	r := newTestRunner(g, root, o)
	g.Expect(r.applyImageOverridesInManifests(
		filepath.Join(root, "operator", "pkg", "manifests"),
	)).To(Succeed())
	g.Expect(r.Stats().ManifestYAMLsImageTextReplaced).To(Equal(1))
	got, err := os.ReadFile(path)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(got)).To(ContainSubstring("quay.io/example/segment-bridge:new"))
}

func TestApplyGitRulesToKustomizationWithRemote(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	kPath := filepath.Join(root, "kustomization.yaml")
	src := `resources:
  - https://github.com/konflux-ci/segment-bridge/config/default?ref=old
images:
  - name: quay.io/konflux-ci/segment-bridge
    newTag: old
`
	g.Expect(os.WriteFile(kPath, []byte(src), 0o644)).To(Succeed())
	r := newTestRunner(g, root, Overrides{
		{
			Name: "segment-bridge",
			Git: []GitRule{
				{
					SourceRepo: "konflux-ci/segment-bridge",
					Remote:     &RemoteGit{Repo: "konflux-ci/segment-bridge", Ref: "newref"},
				},
			},
		},
	})
	written, err := r.applyGitRulesToKustomization(kPath, r.Overrides[0].Git)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(written).To(BeTrue())
	got, err := os.ReadFile(kPath)
	g.Expect(err).ToNot(HaveOccurred())
	text := string(got)
	g.Expect(text).To(ContainSubstring("https://github.com/konflux-ci/segment-bridge/config/default?ref=newref"))
	g.Expect(text).To(ContainSubstring("newTag: newref"))
}

func TestRunnerApplyWithImageOverrideOnly(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	upstreamSegBridge := filepath.Join(root, "operator", "upstream-kustomizations", "segment-bridge")
	g.Expect(os.MkdirAll(upstreamSegBridge, 0o755)).To(Succeed())
	manifestDir := filepath.Join(root, "operator", "pkg", "manifests", "segment-bridge")
	g.Expect(os.MkdirAll(manifestDir, 0o755)).To(Succeed())

	manifestPath := filepath.Join(manifestDir, "manifests.yaml")
	g.Expect(os.WriteFile(manifestPath, []byte(`apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: app
          image: quay.io/konflux-ci/segment-bridge:old
`), 0o644)).To(Succeed())

	r := newTestRunner(g, root, Overrides{
		{
			Name: "segment-bridge",
			Images: []ImageOverride{
				{
					Orig:        "quay.io/konflux-ci/segment-bridge",
					Replacement: "quay.io/example/segment-bridge:new",
				},
			},
		},
	})
	g.Expect(r.Apply()).To(Succeed())
	g.Expect(r.Stats()).To(Equal(ApplyStats{
		ManifestYAMLsImageTextReplaced: 1,
	}))

	gotManifest, err := os.ReadFile(manifestPath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(gotManifest)).To(ContainSubstring("quay.io/example/segment-bridge:new"))

	componentSourcesPath := filepath.Join(root, ".tmp", "component-sources.json")
	componentSources, err := os.ReadFile(componentSourcesPath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(componentSources)).To(ContainSubstring(`"name": "segment-bridge"`))
}

func TestRunnerGitSummaryLines(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	r := Runner{
		Overrides: Overrides{
			{
				Name: "segment-bridge",
				Git: []GitRule{
					{
						SourceRepo: "konflux-ci/segment-bridge",
						Remote:     &RemoteGit{Repo: "konflux-ci/segment-bridge", Ref: "abc123"},
					},
					{
						SourceRepo: "other/org",
						LocalPath:  "/tmp/local-checkout",
					},
				},
			},
		},
	}
	g.Expect(r.GitSummaryLines()).To(Equal([]string{
		"  [segment-bridge] konflux-ci/segment-bridge -> https://github.com/konflux-ci/segment-bridge?ref=abc123",
		"  [segment-bridge] other/org -> local /tmp/local-checkout",
	}))
}

func TestRunnerSummaryLines(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	r := Runner{
		Overrides: Overrides{
			{
				Name: "segment-bridge",
				Images: []ImageOverride{
					{
						Orig:        "quay.io/konflux-ci/segment-bridge",
						Replacement: "quay.io/example/segment-bridge:new",
					},
				},
			},
		},
	}
	g.Expect(r.SummaryLines()).To(Equal([]string{
		"  quay.io/konflux-ci/segment-bridge -> quay.io/example/segment-bridge:new",
	}))
}

// splitImageReference: tag and bare-name cases; digest refs yield empty tag.
func TestSplitImageReference(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		wantName string
		wantTag  string
	}{
		{
			name:     "bare name defaults tag to latest",
			input:    "quay.io/org/app",
			wantName: "quay.io/org/app",
			wantTag:  "latest",
		},
		{
			name:     "tag after last slash colon",
			input:    "quay.io/org/app:mytag",
			wantName: "quay.io/org/app",
			wantTag:  "mytag",
		},
		{
			name:     "port in registry",
			input:    "localhost:5000/org/app:v1",
			wantName: "localhost:5000/org/app",
			wantTag:  "v1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotName, gotTag := splitImageReference(tc.input)
			if gotName != tc.wantName || gotTag != tc.wantTag {
				t.Fatalf("splitImageReference(%q) = (%q, %q), want (%q, %q)",
					tc.input, gotName, gotTag, tc.wantName, tc.wantTag)
			}
		})
	}
}

// Required: a digest reference must not treat the part after @ as a kustomize tag (newTag).
func TestSplitImageReference_digestNotReturnedAsTag(t *testing.T) {
	t.Parallel()
	input := "quay.io/org/app@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, gotTag := splitImageReference(input)
	if strings.HasPrefix(gotTag, "sha256:") {
		t.Fatalf(
			"digest after @ must not be returned as tag string "+
				"(belongs in kustomize digest:, not newTag:); got second return %q",
			gotTag,
		)
	}
}

// applyImageOverridesInKustomizations only runs when an images entry already has digest: (see overrides.go).
// Required: digest replacement must set kustomize digest: and must not put the digest in newTag:.
func TestApplyImageOverridesInKustomizations_digestReplacement(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	upstream := filepath.Join(root, "operator", "upstream-kustomizations", "segment-bridge")
	g.Expect(os.MkdirAll(upstream, 0o755)).To(Succeed())

	const oldDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const newDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	kustomization := `images:
  - name: quay.io/konflux-ci/segment-bridge
    newName: quay.io/konflux-ci/segment-bridge
    digest: ` + oldDigest + `
`
	kPath := filepath.Join(upstream, "kustomization.yaml")
	g.Expect(os.WriteFile(kPath, []byte(kustomization), 0o644)).To(Succeed())

	o := Overrides{
		{
			Name: "segment-bridge",
			Images: []ImageOverride{
				{
					Orig:        "quay.io/konflux-ci/segment-bridge",
					Replacement: "quay.io/example/segment-bridge@" + newDigest,
				},
			},
		},
	}
	r := newTestRunner(g, root, o)
	g.Expect(r.applyImageOverridesInKustomizations(
		filepath.Join(root, "operator", "upstream-kustomizations"),
	)).To(Succeed())
	g.Expect(r.Stats().KustomizationImagesPatched).To(Equal(1))

	got, err := os.ReadFile(kPath)
	g.Expect(err).ToNot(HaveOccurred())
	text := string(got)
	g.Expect(text).To(ContainSubstring("newName: quay.io/example/segment-bridge"))
	g.Expect(text).To(ContainSubstring("digest: " + newDigest))
	g.Expect(text).ToNot(
		ContainSubstring("newTag: "+newDigest),
		"digest must not be written to newTag",
	)
}

// Tag-shaped replacement on an entry that used digest: must move to newName/newTag and drop digest:.
func TestApplyImageOverridesInKustomizations_tagReplacementWhenEntryHasDigest(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	upstream := filepath.Join(root, "operator", "upstream-kustomizations", "segment-bridge")
	g.Expect(os.MkdirAll(upstream, 0o755)).To(Succeed())

	const oldDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	kustomization := `images:
  - name: quay.io/konflux-ci/segment-bridge
    newName: quay.io/konflux-ci/segment-bridge
    digest: ` + oldDigest + `
`
	kPath := filepath.Join(upstream, "kustomization.yaml")
	g.Expect(os.WriteFile(kPath, []byte(kustomization), 0o644)).To(Succeed())

	o := Overrides{
		{
			Name: "segment-bridge",
			Images: []ImageOverride{
				{
					Orig:        "quay.io/konflux-ci/segment-bridge",
					Replacement: "quay.io/example/segment-bridge:pr-override",
				},
			},
		},
	}
	r := newTestRunner(g, root, o)
	g.Expect(r.applyImageOverridesInKustomizations(
		filepath.Join(root, "operator", "upstream-kustomizations"),
	)).To(Succeed())
	g.Expect(r.Stats().KustomizationImagesPatched).To(Equal(1))

	got, err := os.ReadFile(kPath)
	g.Expect(err).ToNot(HaveOccurred())
	text := string(got)
	g.Expect(text).To(ContainSubstring("newName: quay.io/example/segment-bridge"))
	g.Expect(text).To(ContainSubstring("newTag: pr-override"))
	g.Expect(text).ToNot(ContainSubstring("digest: " + oldDigest))
	g.Expect(text).ToNot(ContainSubstring("digest:"), "switching to a tag pin must not leave digest: on this entry")
}

// Only images entries matching orig are updated; others are left unchanged.
func TestApplyImageOverridesInKustomizations_multipleImagesOnlyMatchingRowChanges(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	upstream := filepath.Join(root, "operator", "upstream-kustomizations", "segment-bridge")
	g.Expect(os.MkdirAll(upstream, 0o755)).To(Succeed())

	const firstDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const otherDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	kustomization := `images:
  - name: quay.io/konflux-ci/segment-bridge
    newName: quay.io/konflux-ci/segment-bridge
    digest: ` + firstDigest + `
  - name: quay.io/other/app
    newName: quay.io/other/app
    digest: ` + otherDigest + `
`
	kPath := filepath.Join(upstream, "kustomization.yaml")
	g.Expect(os.WriteFile(kPath, []byte(kustomization), 0o644)).To(Succeed())

	o := Overrides{
		{
			Name: "segment-bridge",
			Images: []ImageOverride{
				{
					Orig:        "quay.io/konflux-ci/segment-bridge",
					Replacement: "quay.io/example/segment-bridge:only-first",
				},
			},
		},
	}
	r := newTestRunner(g, root, o)
	g.Expect(r.applyImageOverridesInKustomizations(
		filepath.Join(root, "operator", "upstream-kustomizations"),
	)).To(Succeed())
	g.Expect(r.Stats().KustomizationImagesPatched).To(Equal(1))

	got, err := os.ReadFile(kPath)
	g.Expect(err).ToNot(HaveOccurred())
	text := string(got)
	g.Expect(text).To(ContainSubstring("newName: quay.io/example/segment-bridge"))
	g.Expect(text).To(ContainSubstring("newTag: only-first"))
	g.Expect(text).To(ContainSubstring("digest: "+otherDigest), "non-matching image entry must keep its digest")
	g.Expect(text).To(ContainSubstring("name: quay.io/other/app"))
}

// Kustomizations that use newTag only (no digest) are skipped by applyImageOverridesInKustomizations entirely.
func TestApplyImageOverridesInKustomizations_skipsWhenNoDigestInFile(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	upstream := filepath.Join(root, "operator", "upstream-kustomizations", "segment-bridge")
	g.Expect(os.MkdirAll(upstream, 0o755)).To(Succeed())

	kustomization := `images:
  - name: quay.io/konflux-ci/segment-bridge
    newName: quay.io/konflux-ci/segment-bridge
    newTag: old
`
	kPath := filepath.Join(upstream, "kustomization.yaml")
	g.Expect(os.WriteFile(kPath, []byte(kustomization), 0o644)).To(Succeed())

	o := Overrides{
		{
			Name: "segment-bridge",
			Images: []ImageOverride{
				{
					Orig:        "quay.io/konflux-ci/segment-bridge",
					Replacement: "quay.io/example/segment-bridge:new",
				},
			},
		},
	}
	r := newTestRunner(g, root, o)
	g.Expect(r.applyImageOverridesInKustomizations(
		filepath.Join(root, "operator", "upstream-kustomizations"),
	)).To(Succeed())
	g.Expect(r.Stats().KustomizationImagesPatched).To(Equal(0))

	got, err := os.ReadFile(kPath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(got)).To(ContainSubstring("newTag: old"))
}

// Manifest replacement substitutes the full replacement string; digest-shaped refs are not split.
func TestApplyImageOverridesInManifests_digestReplacement(t *testing.T) {
	t.Parallel()
	g := NewGomegaWithT(t)

	root := t.TempDir()
	manifestDir := filepath.Join(root, "operator", "pkg", "manifests", "segment-bridge")
	g.Expect(os.MkdirAll(manifestDir, 0o755)).To(Succeed())

	const oldImg = "quay.io/konflux-ci/segment-bridge@sha256:" +
		"1111111111111111111111111111111111111111111111111111111111111111"
	const newImg = "quay.io/example/segment-bridge@sha256:" +
		"2222222222222222222222222222222222222222222222222222222222222222"
	manifest := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: segment-bridge
spec:
  template:
    spec:
      containers:
        - name: app
          image: ` + oldImg + `
`
	path := filepath.Join(manifestDir, "manifests.yaml")
	g.Expect(os.WriteFile(path, []byte(manifest), 0o644)).To(Succeed())

	o := Overrides{
		{
			Name: "segment-bridge",
			Images: []ImageOverride{
				{Orig: "quay.io/konflux-ci/segment-bridge", Replacement: newImg},
			},
		},
	}
	r := newTestRunner(g, root, o)
	g.Expect(r.applyImageOverridesInManifests(
		filepath.Join(root, "operator", "pkg", "manifests"),
	)).To(Succeed())
	g.Expect(r.Stats().ManifestYAMLsImageTextReplaced).To(Equal(1))
	got, err := os.ReadFile(path)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(got)).To(ContainSubstring(newImg))
	g.Expect(string(got)).ToNot(ContainSubstring(oldImg))
}
