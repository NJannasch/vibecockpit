package inventory

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var sensitivePatterns = []struct {
	Name string
	Risk string
}{
	{".env", "Environment variables — may contain API keys, database credentials, or secrets"},
	{".env.local", "Local environment overrides — often contains secrets not in version control"},
	{".env.production", "Production environment — likely contains live credentials"},
	{".env.development", "Development environment — may contain API keys or tokens"},
	{".env.staging", "Staging environment — may contain credentials"},
	{"credentials.json", "Credentials file — may contain OAuth tokens or service account keys"},
	{"service-account.json", "GCP service account key — grants cloud API access"},
	{".npmrc", "npm config — may contain registry auth tokens"},
	{".pypirc", "PyPI config — may contain upload credentials"},
	{".netrc", "Network credentials — stores login info for remote hosts"},
	{".docker/config.json", "Docker config — may contain registry auth tokens"},
}

var sensitiveExtensions = []struct {
	Ext  string
	Risk string
}{
	{".pem", "PEM certificate/key — may contain private keys"},
	{".key", "Private key file"},
	{".p12", "PKCS#12 keystore — contains certificates and private keys"},
	{".pfx", "PFX keystore — contains certificates and private keys"},
	{".jks", "Java keystore — contains certificates and keys"},
}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "vendor": true, ".venv": true,
	"venv": true, "__pycache__": true, ".tox": true, ".mypy_cache": true,
	"dist": true, "build": true, ".next": true, ".nuxt": true,
	".output": true, "target": true, ".gradle": true,
}

const maxWalkDepth = 2

func scanSensitiveFiles(projectPaths []string) []SensitiveFile {
	var found []SensitiveFile

	seen := map[string]bool{}
	for _, pp := range projectPaths {
		if pp == "" || seen[pp] {
			continue
		}
		seen[pp] = true
		projectName := filepath.Base(pp)

		_ = filepath.WalkDir(pp, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			rel, _ := filepath.Rel(pp, path)
			depth := strings.Count(rel, string(os.PathSeparator))
			if d.IsDir() {
				if depth >= maxWalkDepth || skipDirs[d.Name()] {
					return fs.SkipDir
				}
				return nil
			}

			name := d.Name()

			for _, pat := range sensitivePatterns {
				if name == filepath.Base(pat.Name) && strings.HasSuffix(path, pat.Name) {
					found = append(found, SensitiveFile{
						Path:        path,
						Name:        rel,
						ProjectName: projectName,
						Risk:        pat.Risk,
					})
					return nil
				}
			}

			if strings.HasPrefix(name, ".env.") && !matchesKnownEnv(name) && !isEnvTemplate(name) {
				found = append(found, SensitiveFile{
					Path:        path,
					Name:        rel,
					ProjectName: projectName,
					Risk:        "Environment file variant — may contain secrets",
				})
				return nil
			}

			lower := strings.ToLower(name)
			for _, ext := range sensitiveExtensions {
				if strings.HasSuffix(lower, ext.Ext) {
					found = append(found, SensitiveFile{
						Path:        path,
						Name:        rel,
						ProjectName: projectName,
						Risk:        ext.Risk,
					})
					return nil
				}
			}

			return nil
		})
	}

	return found
}

func matchesKnownEnv(name string) bool {
	for _, pat := range sensitivePatterns {
		if filepath.Base(pat.Name) == name {
			return true
		}
	}
	return false
}

func isEnvTemplate(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".example", ".sample", ".template", ".dist"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}
