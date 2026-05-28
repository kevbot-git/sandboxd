package repo

import (
	"testing"
)

func TestParseAddress(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    Address
		wantErr bool
	}{
		// ── Short form (user/repo → github.com) ──────────────────────────
		{
			name:  "short/simple",
			input: "example/sbx-kits",
			want: Address{
				Host:      "github.com",
				User:      "example",
				Repo:      "sbx-kits",
				Kit:       "",
				Key:       "github.com/example/sbx-kits",
				SourceURL: "https://github.com/example/sbx-kits.git",
			},
		},
		{
			name:  "short/with-kit-suffix",
			input: "example/sbx-kits/bun",
			want: Address{
				Host:      "github.com",
				User:      "example",
				Repo:      "sbx-kits",
				Kit:       "bun",
				Key:       "github.com/example/sbx-kits",
				SourceURL: "https://github.com/example/sbx-kits.git",
			},
		},
		{
			name:  "short/hyphens-and-underscores",
			input: "my-org/my_repo",
			want: Address{
				Host:      "github.com",
				User:      "my-org",
				Repo:      "my_repo",
				Key:       "github.com/my-org/my_repo",
				SourceURL: "https://github.com/my-org/my_repo.git",
			},
		},

		// ── Host-prefix form (host.tld/user/repo) ────────────────────────
		{
			name:  "host-prefix/non-github",
			input: "acme.ghe.com/acme-team/sbx-kits",
			want: Address{
				Host:      "acme.ghe.com",
				User:      "acme-team",
				Repo:      "sbx-kits",
				Key:       "acme.ghe.com/acme-team/sbx-kits",
				SourceURL: "https://acme.ghe.com/acme-team/sbx-kits.git",
			},
		},
		{
			name:  "host-prefix/with-kit-suffix",
			input: "acme.ghe.com/acme-team/sbx-kits/node",
			want: Address{
				Host:      "acme.ghe.com",
				User:      "acme-team",
				Repo:      "sbx-kits",
				Kit:       "node",
				Key:       "acme.ghe.com/acme-team/sbx-kits",
				SourceURL: "https://acme.ghe.com/acme-team/sbx-kits.git",
			},
		},

		// ── Full https:// URL form ────────────────────────────────────────
		{
			name:  "url/with-dot-git",
			input: "https://github.com/example/sbx-kits.git",
			want: Address{
				Host:      "github.com",
				User:      "example",
				Repo:      "sbx-kits",
				Key:       "github.com/example/sbx-kits",
				SourceURL: "https://github.com/example/sbx-kits.git",
			},
		},
		{
			name:  "url/without-dot-git",
			input: "https://github.com/example/sbx-kits",
			want: Address{
				Host:      "github.com",
				User:      "example",
				Repo:      "sbx-kits",
				Key:       "github.com/example/sbx-kits",
				SourceURL: "https://github.com/example/sbx-kits.git",
			},
		},
		{
			name:  "url/non-github-host",
			input: "https://example.ghe.io/org/myrepo.git",
			want: Address{
				Host:      "example.ghe.io",
				User:      "org",
				Repo:      "myrepo",
				Key:       "example.ghe.io/org/myrepo",
				SourceURL: "https://example.ghe.io/org/myrepo.git",
			},
		},
		{
			name:  "url/with-kit-suffix",
			input: "https://github.com/example/sbx-kits/bun",
			want: Address{
				Host:      "github.com",
				User:      "example",
				Repo:      "sbx-kits",
				Kit:       "bun",
				Key:       "github.com/example/sbx-kits",
				SourceURL: "https://github.com/example/sbx-kits.git",
			},
		},

		// ── Malformed inputs ─────────────────────────────────────────────
		{
			name:    "malformed/empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "malformed/whitespace-only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "malformed/single-segment",
			input:   "justarepo",
			wantErr: true,
		},
		{
			name:    "malformed/http-url-rejected",
			input:   "http://github.com/example/sbx-kits",
			wantErr: true,
		},
		{
			name:    "malformed/url-only-two-path-segments",
			input:   "https://github.com/example",
			wantErr: true,
		},
		{
			name:    "malformed/short-too-many-segments",
			input:   "user/repo/kit/extra",
			wantErr: true,
		},
		{
			name:    "malformed/host-prefix-too-many-segments",
			input:   "host.com/user/repo/kit/extra",
			wantErr: true,
		},
		{
			name:    "malformed/url-too-many-segments",
			input:   "https://host.com/user/repo/kit/extra",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAddress(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseAddress(%q) = %+v; want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddress(%q) error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseAddress(%q)\ngot:  %+v\nwant: %+v", tc.input, got, tc.want)
			}
		})
	}
}
