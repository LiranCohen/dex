package forgejo

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed theme/*
var themeFS embed.FS

// InstallTheme writes the embedded Dex theme assets into the Forgejo custom
// directory. This includes the CSS theme, logo/favicon SVGs, and template
// injection files. It is safe to call on every startup — files are overwritten
// so theme updates are picked up automatically.
func (c *Config) InstallTheme() error {
	customDir := filepath.Join(c.DataDir, "custom")

	return fs.WalkDir(themeFS, "theme", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Strip the "theme/" prefix to get the relative path inside custom/
		relPath, err := filepath.Rel("theme", path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(customDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		data, err := themeFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded theme file %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create dir for %s: %w", destPath, err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write theme file %s: %w", destPath, err)
		}

		return nil
	})
}
