// Copyright 2019 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRepoSpecFromUrl_Permute(t *testing.T) {
	// Generate all many permutations of host, RepoPath, pathName, and ref.
	// Not all combinations make sense, but the parsing is very permissive and
	// we probably stil don't want to break backwards compatibility for things
	// that are unintentionally supported.
	var schemeAuthority = []struct{ raw, normalized string }{
		{"gitHub.com/", "https://github.com/"},
		{"github.com:", "https://github.com/"},
		{"http://github.com/", "https://github.com/"},
		{"https://github.com/", "https://github.com/"},
		{"hTTps://github.com/", "https://github.com/"},
		{"https://git-codecommit.us-east-2.amazonaws.com/", "https://git-codecommit.us-east-2.amazonaws.com/"},
		{"https://fabrikops2.visualstudio.com/", "https://fabrikops2.visualstudio.com/"},
		{"ssh://git.example.com:7999/", "ssh://git.example.com:7999/"},
		{"git::https://gitlab.com/", "https://gitlab.com/"},
		{"git::http://git.example.com/", "http://git.example.com/"},
		{"git::https://git.example.com/", "https://git.example.com/"},
		{"git@github.com:", "git@github.com:"},
		{"git@github.com/", "git@github.com:"},
		{"git::git@github.com:", "git@github.com:"},
	}
	var repoPaths = []string{"someOrg/someRepo", "kubernetes/website"}
	var pathNames = []string{"README.md", "foo/krusty.txt", ""}
	var refArgs = []string{"group/version", "someBranch", "master", "v0.1.0", ""}

	makeURL := func(hostFmt, repoPath, path, ref string) string {
		if len(path) > 0 {
			repoPath = filepath.Join(repoPath, path)
		}
		url := hostFmt + repoPath
		if ref != "" {
			url += refQuery + ref
		}
		return url
	}

	var i int
	for _, v := range schemeAuthority {
		hostRaw := v.raw
		hostSpec := v.normalized
		for _, repoPath := range repoPaths {
			for _, pathName := range pathNames {
				for _, hrefArg := range refArgs {
					t.Run(fmt.Sprintf("t%d", i), func(t *testing.T) {
						uri := makeURL(hostRaw, repoPath, pathName, hrefArg)
						rs, err := NewRepoSpecFromURL(uri)
						require.NoErrorf(t, err, "unexpected error creating RepoSpec for uri %s", uri)
						assert.Equal(t, hostSpec, rs.Host, "unexpected host for uri %s", uri)
						assert.Equal(t, repoPath, rs.RepoPath, "unexpected RepoPath for uri %s", uri)
						assert.Equal(t, pathName, rs.KustRootPath, "unexpected KustRootPath for uri %s", uri)
						assert.Equal(t, hrefArg, rs.Ref, "unexpected ref for uri %s", uri)
					})
					i++
				}
			}
		}
	}
}

func TestNewRepoSpecFromUrlErrors(t *testing.T) {
	badData := map[string]struct {
		url, error string
	}{
		"absolute_path": {
			"/tmp",
			"uri looks like abs path",
		},
		"relative path": {
			"../../tmp",
			"failed to parse scheme",
		},
		"local path that looks somewhat like a github url": {
			"src/github.com/org/repo/path",
			"failed to parse scheme",
		},
		"no_slashes": {
			"iauhsdiuashduas",
			"failed to parse scheme",
		},
		"bad_scheme": {
			"htxxxtp://github.com/",
			"failed to parse scheme",
		},
		"no_org_repo": {
			"ssh://git.example.com",
			"failed to parse repo path segment",
		},
		"hashicorp_git_only": {
			"git::___",
			"failed to parse scheme",
		},
		"query_after_host": {
			"https://host?ref=group/version/minor_version",
			"failed to parse repo path segment",
		},
		"path_exits_repo": {
			"https://github.com/org/repo.git//path/../../exits/repo",
			"url path exits repo",
		},
		"bad github separator": {
			"github.com!org/repo.git//path",
			"failed to parse scheme",
		},
		"mysterious gh: prefix previously supported is no longer handled": {
			"gh:org/repo",
			"failed to parse scheme",
		},
		"invalid Github url missing orgrepo": {
			"https://github.com/thisisa404.yaml",
			"failed to parse repo path segment",
		},
		"file protocol with excessive slashes": { // max valid is three: two for the scheme and one for the root
			"file:////tmp//path/to/whatever",
			"failed to parse repo path segment",
		},
		"unsupported protocol after username (invalid)": {
			"git@scp://github.com/org/repo.git//path",
			"failed to parse repo path segment",
		},
		"supported protocol after username (invalid)": {
			"git@ssh://github.com/org/repo.git//path",
			"failed to parse repo path segment",
		},
	}

	for name, testCase := range badData {
		t.Run(name, func(t *testing.T) {
			_, err := NewRepoSpecFromURL(testCase.url)
			require.Error(t, err)
			require.Contains(t, err.Error(), testCase.error)
		})
	}
}

func TestNewRepoSpecFromUrl_Smoke(t *testing.T) {
	// A set of end to end parsing tests that smoke out obvious issues
	// No tests for submodules and timeout as the expectations are hard-coded
	// to the defaults for compactness.
	testcases := []struct {
		name      string
		input     string
		repoSpec  RepoSpec
		cloneSpec string
		absPath   string
	}{
		{
			name:      "t1",
			input:     "https://git-codecommit.us-east-2.amazonaws.com/someorg/somerepo/somedir",
			cloneSpec: "https://git-codecommit.us-east-2.amazonaws.com/someorg/somerepo",
			absPath:   notCloned.Join("somedir"),
			repoSpec: RepoSpec{
				Host:         "https://git-codecommit.us-east-2.amazonaws.com/",
				RepoPath:     "someorg/somerepo",
				KustRootPath: "somedir",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "t2",
			input:     "https://git-codecommit.us-east-2.amazonaws.com/someorg/somerepo/somedir?ref=testbranch",
			cloneSpec: "https://git-codecommit.us-east-2.amazonaws.com/someorg/somerepo",
			absPath:   notCloned.Join("somedir"),
			repoSpec: RepoSpec{
				Host:         "https://git-codecommit.us-east-2.amazonaws.com/",
				RepoPath:     "someorg/somerepo",
				KustRootPath: "somedir",
				Ref:          "testbranch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "t3",
			input:     "https://fabrikops2.visualstudio.com/someorg/somerepo?ref=master",
			cloneSpec: "https://fabrikops2.visualstudio.com/someorg/somerepo",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "https://fabrikops2.visualstudio.com/",
				RepoPath:  "someorg/somerepo",
				Ref:       "master",
				GitSuffix: ".git",
			},
		},
		{
			name:      "t4",
			input:     "http://github.com/someorg/somerepo/somedir",
			cloneSpec: "https://github.com/someorg/somerepo.git",
			absPath:   notCloned.Join("somedir"),
			repoSpec: RepoSpec{
				Host:         "https://github.com/",
				RepoPath:     "someorg/somerepo",
				KustRootPath: "somedir",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "t5",
			input:     "git@github.com:someorg/somerepo/somedir",
			cloneSpec: "git@github.com:someorg/somerepo.git",
			absPath:   notCloned.Join("somedir"),
			repoSpec: RepoSpec{
				Host:         "git@github.com:",
				RepoPath:     "someorg/somerepo",
				KustRootPath: "somedir",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "non-github_scp",
			input:     "git@gitlab2.sqtools.ru:infra/kubernetes/thanos-base.git?ref=v0.1.0",
			cloneSpec: "git@gitlab2.sqtools.ru:infra/kubernetes/thanos-base.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "git@gitlab2.sqtools.ru:",
				RepoPath:  "infra/kubernetes/thanos-base",
				Ref:       "v0.1.0",
				GitSuffix: ".git",
			},
		},
		{
			name:      "non-github_scp with path delimiter",
			input:     "git@bitbucket.org:company/project.git//path?ref=branch",
			cloneSpec: "git@bitbucket.org:company/project.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "git@bitbucket.org:",
				RepoPath:     "company/project",
				KustRootPath: "path",
				Ref:          "branch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "non-github_scp incorrectly using slash (invalid but currently passed through to git)",
			input:     "git@bitbucket.org/company/project.git//path?ref=branch",
			cloneSpec: "git@bitbucket.org/company/project.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "git@bitbucket.org/",
				RepoPath:     "company/project",
				KustRootPath: "path",
				Ref:          "branch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "non-github_git-user_ssh",
			input:     "ssh://git@bitbucket.org/company/project.git//path?ref=branch",
			cloneSpec: "ssh://git@bitbucket.org/company/project.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "ssh://git@bitbucket.org/",
				RepoPath:     "company/project",
				KustRootPath: "path",
				Ref:          "branch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "_git host delimiter in non-github url",
			input:     "https://itfs.mycompany.com/collection/project/_git/somerepos",
			cloneSpec: "https://itfs.mycompany.com/collection/project/_git/somerepos",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:     "https://itfs.mycompany.com/",
				RepoPath: "collection/project/_git/somerepos",
			},
		},
		{
			name:      "_git host delimiter in non-github url with params",
			input:     "https://itfs.mycompany.com/collection/project/_git/somerepos?version=v1.0.0",
			cloneSpec: "https://itfs.mycompany.com/collection/project/_git/somerepos",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:     "https://itfs.mycompany.com/",
				RepoPath: "collection/project/_git/somerepos",
				Ref:      "v1.0.0",
			},
		},
		{
			name:      "_git host delimiter in non-github url with kust root path and params",
			input:     "https://itfs.mycompany.com/collection/project/_git/somerepos/somedir?version=v1.0.0",
			cloneSpec: "https://itfs.mycompany.com/collection/project/_git/somerepos",
			absPath:   notCloned.Join("somedir"),
			repoSpec: RepoSpec{
				Host:         "https://itfs.mycompany.com/",
				RepoPath:     "collection/project/_git/somerepos",
				KustRootPath: "somedir",
				Ref:          "v1.0.0",
			},
		},
		{
			name:      "_git host delimiter in non-github url with no kust root path",
			input:     "git::https://itfs.mycompany.com/collection/project/_git/somerepos",
			cloneSpec: "https://itfs.mycompany.com/collection/project/_git/somerepos",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:     "https://itfs.mycompany.com/",
				RepoPath: "collection/project/_git/somerepos",
			},
		},
		{
			name:      "t12",
			input:     "https://bitbucket.example.com/scm/project/repository.git",
			cloneSpec: "https://bitbucket.example.com/scm/project/repository.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "https://bitbucket.example.com/",
				RepoPath:  "scm/project/repository",
				GitSuffix: ".git",
			},
		},
		{
			name:      "t13",
			input:     "ssh://git-codecommit.us-east-2.amazonaws.com/someorg/somerepo/somepath",
			cloneSpec: "ssh://git-codecommit.us-east-2.amazonaws.com/someorg/somerepo",
			absPath:   notCloned.Join("somepath"),
			repoSpec: RepoSpec{
				Host:         "ssh://git-codecommit.us-east-2.amazonaws.com/",
				RepoPath:     "someorg/somerepo",
				KustRootPath: "somepath",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "t14",
			input:     "git@github.com/someorg/somerepo/somepath",
			cloneSpec: "git@github.com:someorg/somerepo.git",
			absPath:   notCloned.Join("somepath"),
			repoSpec: RepoSpec{
				Host:         "git@github.com:",
				RepoPath:     "someorg/somerepo",
				KustRootPath: "somepath",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "t15",
			input:     "https://github.com/kubernetes-sigs/kustomize//examples/multibases/dev/?ref=v1.0.6",
			cloneSpec: "https://github.com/kubernetes-sigs/kustomize.git",
			absPath:   notCloned.Join("/examples/multibases/dev"),
			repoSpec: RepoSpec{
				Host:         "https://github.com/",
				RepoPath:     "kubernetes-sigs/kustomize",
				KustRootPath: "examples/multibases/dev/",
				Ref:          "v1.0.6",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "file protocol with git-suffixed repo path and params",
			input:     "file://a/b/c/someRepo.git/somepath?ref=someBranch",
			cloneSpec: "file://a/b/c/someRepo.git",
			absPath:   notCloned.Join("somepath"),
			repoSpec: RepoSpec{
				Host:         "file://",
				RepoPath:     "a/b/c/someRepo",
				KustRootPath: "somepath",
				Ref:          "someBranch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "file protocol with two slashes, with kust root path and params",
			input:     "file://a/b/c/someRepo//somepath?ref=someBranch",
			cloneSpec: "file://a/b/c/someRepo",
			absPath:   notCloned.Join("somepath"),
			repoSpec: RepoSpec{
				Host:         "file://",
				RepoPath:     "a/b/c/someRepo",
				KustRootPath: "somepath",
				Ref:          "someBranch",
			},
		},
		{
			name:      "file protocol with two slashes, with ref and no kust root path",
			input:     "file://a/b/c/someRepo?ref=someBranch",
			cloneSpec: "file://a/b/c/someRepo",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:     "file://",
				RepoPath: "a/b/c/someRepo",
				Ref:      "someBranch",
			},
		},
		{
			name:      "file protocol with three slashes, with ref and no kust root path",
			input:     "file:///a/b/c/someRepo?ref=someBranch",
			cloneSpec: "file:///a/b/c/someRepo",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:     "file://",
				RepoPath: "/a/b/c/someRepo",
				Ref:      "someBranch",
			},
		},
		{
			name:      "t20",
			input:     "ssh://git@github.com/kubernetes-sigs/kustomize//examples/multibases/dev?ref=v1.0.6",
			cloneSpec: "git@github.com:kubernetes-sigs/kustomize.git",
			absPath:   notCloned.Join("examples/multibases/dev"),
			repoSpec: RepoSpec{
				Host:         "git@github.com:",
				RepoPath:     "kubernetes-sigs/kustomize",
				KustRootPath: "examples/multibases/dev",
				Ref:          "v1.0.6",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "file protocol with three slashes, no kust root path or params",
			input:     "file:///a/b/c/someRepo",
			cloneSpec: "file:///a/b/c/someRepo",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:     "file://",
				RepoPath: "/a/b/c/someRepo",
			},
		},
		{
			name:      "file protocol with three slashes, no repo or kust root path or params",
			input:     "file:///",
			cloneSpec: "file:///",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:     "file://",
				RepoPath: "/",
			},
		},
		{
			name:      "double-slash path delimiter https",
			input:     "https://fake-git-hosting.org/path/to/repo//examples/multibases/dev",
			cloneSpec: "https://fake-git-hosting.org/path/to/repo.git",
			absPath:   notCloned.Join("/examples/multibases/dev"),
			repoSpec: RepoSpec{
				Host:         "https://fake-git-hosting.org/",
				RepoPath:     "path/to/repo",
				KustRootPath: "examples/multibases/dev",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "double-slash path delimeter ssh",
			input:     "ssh://alice@acme.co/path/to/repo//examples/multibases/dev",
			cloneSpec: "ssh://alice@acme.co/path/to/repo.git",
			absPath:   notCloned.Join("/examples/multibases/dev"),
			repoSpec: RepoSpec{
				Host:         "ssh://alice@acme.co/",
				RepoPath:     "path/to/repo",
				KustRootPath: "examples/multibases/dev",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "query_slash",
			input:     "https://authority/org/repo?ref=group/version",
			cloneSpec: "https://authority/org/repo.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "https://authority/",
				RepoPath:  "org/repo",
				Ref:       "group/version",
				GitSuffix: ".git",
			},
		},
		{
			name:      "query_git_delimiter",
			input:     "https://authority/org/repo/?ref=includes_git/for_some_reason",
			cloneSpec: "https://authority/org/repo.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "https://authority/",
				RepoPath:  "org/repo",
				Ref:       "includes_git/for_some_reason",
				GitSuffix: ".git",
			},
		},
		{
			name:      "query_git_suffix",
			input:     "https://authority/org/repo/?ref=includes.git/for_some_reason",
			cloneSpec: "https://authority/org/repo.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "https://authority/",
				RepoPath:  "org/repo",
				Ref:       "includes.git/for_some_reason",
				GitSuffix: ".git",
			},
		},
		{
			name:      "non_parsable_path",
			input:     "https://authority/org/repo/%-invalid-uri-so-not-parsable-by-net/url.Parse",
			cloneSpec: "https://authority/org/repo.git",
			absPath:   notCloned.Join("%-invalid-uri-so-not-parsable-by-net/url.Parse"),
			repoSpec: RepoSpec{
				Host:         "https://authority/",
				RepoPath:     "org/repo",
				KustRootPath: "%-invalid-uri-so-not-parsable-by-net/url.Parse",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "non-git username with non-github host",
			input:     "ssh://myusername@bitbucket.org/ourteamname/ourrepositoryname.git//path?ref=branch",
			cloneSpec: "ssh://myusername@bitbucket.org/ourteamname/ourrepositoryname.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "ssh://myusername@bitbucket.org/",
				RepoPath:     "ourteamname/ourrepositoryname",
				KustRootPath: "path",
				Ref:          "branch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "username-like filepath with file protocol",
			input:     "file://git@home/path/to/repository.git//path?ref=branch",
			cloneSpec: "file://git@home/path/to/repository.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "file://",
				RepoPath:     "git@home/path/to/repository",
				KustRootPath: "path",
				Ref:          "branch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "username with http protocol (invalid but currently passed through to git)",
			input:     "http://git@home.com/path/to/repository.git//path?ref=branch",
			cloneSpec: "http://git@home.com/path/to/repository.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "http://git@home.com/",
				RepoPath:     "path/to/repository",
				KustRootPath: "path",
				Ref:          "branch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "username with https protocol (invalid but currently passed through to git)",
			input:     "https://git@home.com/path/to/repository.git//path?ref=branch",
			cloneSpec: "https://git@home.com/path/to/repository.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "https://git@home.com/",
				RepoPath:     "path/to/repository",
				KustRootPath: "path",
				Ref:          "branch",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "complex github ssh url from docs",
			input:     "ssh://git@ssh.github.com:443/YOUR-USERNAME/YOUR-REPOSITORY.git",
			cloneSpec: "ssh://git@ssh.github.com:443/YOUR-USERNAME/YOUR-REPOSITORY.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:         "ssh://git@ssh.github.com:443/",
				RepoPath:     "YOUR-USERNAME/YOUR-REPOSITORY",
				KustRootPath: "",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "colon behind slash not scp delimiter",
			input:     "git@gitlab.com/user:name/YOUR-REPOSITORY.git/path",
			cloneSpec: "git@gitlab.com/user:name/YOUR-REPOSITORY.git",
			absPath:   notCloned.Join("path"),
			repoSpec: RepoSpec{
				Host:         "git@gitlab.com/",
				RepoPath:     "user:name/YOUR-REPOSITORY",
				KustRootPath: "path",
				GitSuffix:    ".git",
			},
		},
		{
			name:      "gitlab URLs with explicit git suffix",
			input:     "git@gitlab.com:gitlab-tests/sample-project.git",
			cloneSpec: "git@gitlab.com:gitlab-tests/sample-project.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "git@gitlab.com:",
				RepoPath:  "gitlab-tests/sample-project",
				GitSuffix: ".git",
			},
		},
		{
			name:      "gitlab URLs without explicit git suffix",
			input:     "git@gitlab.com:gitlab-tests/sample-project",
			cloneSpec: "git@gitlab.com:gitlab-tests/sample-project.git",
			absPath:   notCloned.String(),
			repoSpec: RepoSpec{
				Host:      "git@gitlab.com:",
				RepoPath:  "gitlab-tests/sample-project",
				GitSuffix: ".git",
			},
		},
		{
			name:      "azure host with _git and // path separator",
			input:     "https://username@dev.azure.com/org/project/_git/repo//path/to/kustomization/root",
			cloneSpec: "https://username@dev.azure.com/org/project/_git/repo",
			absPath:   notCloned.Join("path/to/kustomization/root"),
			repoSpec: RepoSpec{
				Host:         "https://username@dev.azure.com/",
				RepoPath:     "org/project/_git/repo",
				KustRootPath: "path/to/kustomization/root",
				GitSuffix:    "",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			rs, err := NewRepoSpecFromURL(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.cloneSpec, rs.CloneSpec(), "cloneSpec mismatch")
			assert.Equal(t, tc.absPath, rs.AbsPath(), "absPath mismatch")
			// some values have defaults. Clear them here so test cases remain compact.
			// This means submodules and timeout cannot be tested here. That's fine since
			// they are tested in TestParseQuery.
			rs.raw = ""
			rs.Dir = ""
			rs.Submodules = false
			rs.Timeout = 0
			assert.Equal(t, &tc.repoSpec, rs)
		})
	}
}

func TestNewRepoSpecFromURL_DefaultQueryParams(t *testing.T) {
	repoSpec, err := NewRepoSpecFromURL("https://github.com/org/repo")
	require.NoError(t, err)
	require.Equal(t, defaultSubmodules, repoSpec.Submodules)
	require.Equal(t, defaultTimeout, repoSpec.Timeout)
}

func TestIsAzureHost(t *testing.T) {
	testcases := []struct {
		input  string
		expect bool
	}{
		{
			input:  "https://git-codecommit.us-east-2.amazonaws.com",
			expect: false,
		},
		{
			input:  "ssh://git-codecommit.us-east-2.amazonaws.com",
			expect: false,
		},
		{
			input:  "https://fabrikops2.visualstudio.com/",
			expect: true,
		},
		{
			input:  "https://dev.azure.com/myorg/myproject/",
			expect: true,
		},
	}
	for _, testcase := range testcases {
		actual := isAzureHost(testcase.input)
		if actual != testcase.expect {
			t.Errorf("IsAzureHost: expected %v, but got %v on %s", testcase.expect, actual, testcase.input)
		}
	}
}

func TestParseQuery(t *testing.T) {
	testcases := []struct {
		name       string
		input      string
		ref        string
		submodules bool
		timeout    time.Duration
	}{
		{
			name:       "empty",
			input:      "",
			ref:        "",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name:       "ref",
			input:      "ref=v1.0.0",
			ref:        "v1.0.0",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name:       "ref_slash",
			input:      "ref=kustomize/v4.5.7",
			ref:        "kustomize/v4.5.7",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name:       "version",
			input:      "version=master",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name: "ref_and_version",
			// A ref value takes precedence over a version value.
			input:      "version=master&ref=v1.0.0",
			ref:        "v1.0.0",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name: "empty_submodules",
			// Empty submodules value uses default.
			input:      "version=master&submodules=",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name: "bad_submodules",
			// Malformed submodules value uses default.
			input:      "version=master&submodules=maybe",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name:       "submodules_true",
			input:      "version=master&submodules=true",
			ref:        "master",
			submodules: true,
			timeout:    defaultTimeout,
		},
		{
			name:       "submodules_false",
			input:      "version=master&submodules=false",
			ref:        "master",
			submodules: false,
			timeout:    defaultTimeout,
		},
		{
			name: "empty_timeout",
			// Empty timeout value uses default.
			input:      "version=master&timeout=",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name: "bad_timeout",
			// Malformed timeout value uses default.
			input:      "version=master&timeout=jiffy",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name: "zero_timeout",
			// Zero timeout value uses default.
			input:      "version=master&timeout=0",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name:       "zero_unit_timeout",
			input:      "version=master&timeout=0s",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    defaultTimeout,
		},
		{
			name:       "timeout",
			input:      "version=master&timeout=61",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    61 * time.Second,
		},
		{
			name:       "timeout_unit",
			input:      "version=master&timeout=1m1s",
			ref:        "master",
			submodules: defaultSubmodules,
			timeout:    61 * time.Second,
		},
		{
			name:       "all",
			input:      "version=master&submodules=false&timeout=1m1s",
			ref:        "master",
			submodules: false,
			timeout:    61 * time.Second,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ref, timeout, submodules := parseQuery(tc.input)
			assert.Equal(t, tc.ref, ref, "ref mismatch")
			assert.Equal(t, tc.timeout, timeout, "timeout mismatch")
			assert.Equal(t, tc.submodules, submodules, "submodules mismatch")
		})
	}
}

func TestIsAWSHost(t *testing.T) {
	testcases := []struct {
		input  string
		expect bool
	}{
		{
			input:  "https://git-codecommit.us-east-2.amazonaws.com",
			expect: true,
		},
		{
			input:  "ssh://git-codecommit.us-east-2.amazonaws.com",
			expect: true,
		},
		{
			input:  "git@github.com:",
			expect: false,
		},
		{
			input:  "http://github.com/",
			expect: false,
		},
	}
	for _, testcase := range testcases {
		actual := isAWSHost(testcase.input)
		if actual != testcase.expect {
			t.Errorf("IsAWSHost: expected %v, but got %v on %s", testcase.expect, actual, testcase.input)
		}
	}
}
