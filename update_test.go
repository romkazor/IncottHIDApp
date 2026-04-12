package main

import "testing"

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"1.0.0", "1.0.0", 0},
		{"v1.0.0", "v1.0.1", -1},
		{"v1.0.1", "v1.0.0", 1},
		{"v1.0.0", "v1.1.0", -1},
		{"v2.0.0", "v1.9.9", 1},
		{"v0.1.0", "v0.2.0", -1},
		{"v0.2.0", "v0.1.0", 1},
		{"v10.0.0", "v9.0.0", 1},    // numeric compare, not lexicographic
		{"v1.0.0-rc1", "v1.0.0", 0}, // pre-release stripped
		{"dev", "v1.0.0", 0},        // unparseable -> 0
		{"v1.0.0", "dev", 0},
		{"garbage", "more garbage", 0},
	}
	for _, tt := range tests {
		got := compareSemver(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsValidRepo(t *testing.T) {
	tests := []struct {
		repo string
		want bool
	}{
		{"", false},
		{"romkazor/IncottHIDApp", true},
		{"user/repo", true},
		{"user.name/repo.name", true},
		{"user_1/repo-2", true},
		{"no-slash", false},
		{"/missing-owner", false},
		{"missing-repo/", false},
		{"too/many/slashes", false},
		{"../../evil/path", false},
		{"user/repo?query", false},
		{"user/repo#frag", false},
		{"user/repo@branch", false},
		{"user /repo", false}, // space not allowed
	}
	for _, tt := range tests {
		got := isValidRepo(tt.repo)
		if got != tt.want {
			t.Errorf("isValidRepo(%q) = %v, want %v", tt.repo, got, tt.want)
		}
	}
}

func TestBuildReleasesAPIURL(t *testing.T) {
	tests := []struct {
		repo string
		want string
	}{
		{"romkazor/IncottHIDApp", "https://api.github.com/repos/romkazor/IncottHIDApp/releases/latest"},
		{"user/MyFork", "https://api.github.com/repos/user/MyFork/releases/latest"},
		{"", ""},
		{"garbage", ""},
		{"too/many/slashes", ""},
	}
	for _, tt := range tests {
		got := buildReleasesAPIURL(tt.repo)
		if got != tt.want {
			t.Errorf("buildReleasesAPIURL(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestParseSemver(t *testing.T) {
	tests := []struct {
		v        string
		want     [3]int
		wantOK   bool
	}{
		{"v1.2.3", [3]int{1, 2, 3}, true},
		{"1.2.3", [3]int{1, 2, 3}, true},
		{"v0.0.0", [3]int{0, 0, 0}, true},
		{"v10.20.30", [3]int{10, 20, 30}, true},
		{"v1.2.3-rc1", [3]int{1, 2, 3}, true},
		{"v1.2.3+build", [3]int{1, 2, 3}, true},
		{"v1.2", [3]int{}, false},
		{"v1", [3]int{}, false},
		{"dev", [3]int{}, false},
		{"", [3]int{}, false},
	}
	for _, tt := range tests {
		got, ok := parseSemver(tt.v)
		if ok != tt.wantOK {
			t.Errorf("parseSemver(%q) ok = %v, want %v", tt.v, ok, tt.wantOK)
		}
		if ok && got != tt.want {
			t.Errorf("parseSemver(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}
