package buildconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// releaseConfig is the part of the release config the tests look at.
type releaseConfig struct {
	Builds []struct {
		Targets []string
		Ldflags []string
	}
	Archives []struct {
		Formats         []string
		FormatOverrides []struct {
			Goos    string
			Formats []string
		} `yaml:"format_overrides"`
	}
	Nfpms []struct {
		Formats    []string
		Maintainer string
		Contents   []struct {
			Src string
			Dst string
		}
	}
	HomebrewCasks []struct {
		SkipUpload string `yaml:"skip_upload"`
		Repository struct {
			Owner  string
			Name   string
			Branch string
			Token  string
		}
		Hooks map[string]any
	} `yaml:"homebrew_casks"`
	Notarize struct {
		Macos []struct {
			Enabled string
			Sign    struct {
				Certificate string
				Password    string
			}
			Notarize struct {
				IssuerID string `yaml:"issuer_id"`
				KeyID    string `yaml:"key_id"`
				Key      string
				Wait     bool
			}
		}
	}
	Changelog struct {
		Use string
	}
	Release struct {
		Draft bool
	}
}

// release reads the config that turns a tag into a release.
func release(t *testing.T) releaseConfig {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", ".goreleaser.yaml"))
	require.NoError(t, err)

	var cfg releaseConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg))

	return cfg
}

func TestRelease_BuildsEveryPlatform(t *testing.T) {
	r := require.New(t)

	cfg := release(t)
	r.Len(cfg.Builds, 1)

	// A target is goos_goarch plus the CPU level; the platform is the first two.
	var platforms []string
	for _, target := range cfg.Builds[0].Targets {
		parts := strings.SplitN(target, "_", 3)
		platforms = append(platforms, parts[0]+"/"+parts[1])
	}

	// urga is downloaded for the machine it will run on, so every one of
	// them is built.
	r.ElementsMatch([]string{
		"linux/amd64",
		"linux/arm64",
		"linux/arm",
		"darwin/amd64",
		"darwin/arm64",
		"windows/amd64",
		"windows/arm64",
		"freebsd/amd64",
	}, platforms)
}

func TestRelease_StampsTheVersion(t *testing.T) {
	r := require.New(t)

	cfg := release(t)
	r.Len(cfg.Builds, 1)

	ldflags := strings.Join(cfg.Builds[0].Ldflags, " ")

	// The linker stamps the tag, commit and date of the build into a release.
	// The tag keeps its v, the way the header shows it.
	const pkg = "-X github.com/ingvarch/urga/internal/version."
	r.Contains(ldflags, pkg+"Version=v{{ .Version }}")
	r.Contains(ldflags, pkg+"Commit=")
	r.Contains(ldflags, pkg+"Date=")
}

func TestRelease_PacksDebAndRpm(t *testing.T) {
	r := require.New(t)

	cfg := release(t)
	r.Len(cfg.Nfpms, 1)
	pkg := cfg.Nfpms[0]

	// Linux installs urga with its own package manager.
	r.ElementsMatch([]string{"deb", "rpm"}, pkg.Formats)
	r.NotEmpty(pkg.Maintainer)

	// MIT asks for the license in every copy, and a package is a copy.
	var license string
	for _, file := range pkg.Contents {
		if file.Src == "LICENSE" {
			license = file.Dst
		}
	}
	r.Equal("/usr/share/doc/urga/LICENSE", license)
}

func TestRelease_ZipsForWindows(t *testing.T) {
	r := require.New(t)

	cfg := release(t)
	r.Len(cfg.Archives, 1)
	archive := cfg.Archives[0]

	// Windows opens a zip with nothing extra installed, the rest a tarball.
	r.Equal([]string{"tar.gz"}, archive.Formats)
	r.Len(archive.FormatOverrides, 1)
	r.Equal("windows", archive.FormatOverrides[0].Goos)
	r.Equal([]string{"zip"}, archive.FormatOverrides[0].Formats)
}

func TestRelease_PublishesTheCaskToTheTap(t *testing.T) {
	r := require.New(t)

	cfg := release(t)
	r.Len(cfg.HomebrewCasks, 1)
	cask := cfg.HomebrewCasks[0]

	// brew install ingvarch/tap/urga reads the tap, so the cask goes there.
	r.Equal("ingvarch", cask.Repository.Owner)
	r.Equal("homebrew-tap", cask.Repository.Name)
	r.Equal("main", cask.Repository.Branch)

	// The job's own token cannot push to another repository.
	r.Equal("{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}", cask.Repository.Token)

	// A release candidate stays out of brew upgrade.
	r.Equal("auto", cask.SkipUpload)
}

func TestRelease_SignsAndNotarizesForMacOS(t *testing.T) {
	r := require.New(t)

	cfg := release(t)
	r.Len(cfg.Notarize.Macos, 1)
	macos := cfg.Notarize.Macos[0]

	// Gatekeeper runs a downloaded binary only when Apple has notarized it.
	// A snapshot has no Apple keys; a release without them fails instead of
	// shipping a binary that macOS refuses.
	r.Equal("{{ not .IsSnapshot }}", macos.Enabled)

	// The keys live in the repository secrets, never in the tree.
	r.Equal("{{ .Env.MACOS_SIGN_P12 }}", macos.Sign.Certificate)
	r.Equal("{{ .Env.MACOS_SIGN_PASSWORD }}", macos.Sign.Password)
	r.Equal("{{ .Env.MACOS_NOTARY_ISSUER_ID }}", macos.Notarize.IssuerID)
	r.Equal("{{ .Env.MACOS_NOTARY_KEY_ID }}", macos.Notarize.KeyID)
	r.Equal("{{ .Env.MACOS_NOTARY_KEY }}", macos.Notarize.Key)

	// A rejected binary stops the release before anything is published.
	r.True(macos.Notarize.Wait)
}

func TestRelease_CaskRunsNothingOnInstall(t *testing.T) {
	r := require.New(t)

	cfg := release(t)
	r.Len(cfg.HomebrewCasks, 1)

	// A notarized binary needs no quarantine removal, and Homebrew has
	// deprecated the Ruby hooks a cask would run it in.
	r.Empty(cfg.HomebrewCasks[0].Hooks)
}

func TestRelease_TakesTheNotesFromGitHub(t *testing.T) {
	// GitHub lists the merged pull requests and who wrote them, which the
	// commit log alone does not.
	require.Equal(t, "github-native", release(t).Changelog.Use)
}

func TestRelease_IsPublishedAtOnce(t *testing.T) {
	// The cask points at the release files; a draft would serve brew a 404
	// until somebody published it by hand.
	require.False(t, release(t).Release.Draft)
}
