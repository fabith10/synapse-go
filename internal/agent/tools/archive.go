package agenttools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fabith10/synapse-go/adk"
)

// GetArchiveManagerTool returns a tool to create, extract, or list .zip and .tar.gz archives.
func GetArchiveManagerTool() adk.Tool {
	return adk.Tool{
		Name:        "archive_manager",
		Description: "Creates, extracts, or lists contents of .zip and .tar.gz archives.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "Action: 'create', 'extract', 'list'",
				},
				"archive_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the archive file (.zip or .tar.gz)",
				},
				"target_dir": map[string]interface{}{
					"type":        "string",
					"description": "Target directory for extraction or root dir for creation",
				},
				"files": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of file paths to include when creating an archive",
				},
			},
			"required": []interface{}{"action", "archive_path"},
		},
		Tier: adk.TierNative,
		Execute: func(ctx context.Context, args []byte) (string, error) {
			var params struct {
				Action      string   `json:"action"`
				ArchivePath string   `json:"archive_path"`
				TargetDir   string   `json:"target_dir"`
				Files       []string `json:"files"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("invalid arguments: %w", err)
			}

			if params.ArchivePath == "" {
				return "", fmt.Errorf("archive_path is required")
			}

			archivePath := params.ArchivePath
			targetDir := params.TargetDir

			if root := ctx.Value(WorkspaceRootKey); root != nil {
				if rStr, ok := root.(string); ok && rStr != "" {
					if !filepath.IsAbs(archivePath) {
						archivePath = filepath.Join(rStr, archivePath)
					}
					if targetDir == "" {
						targetDir = rStr
					} else if !filepath.IsAbs(targetDir) {
						targetDir = filepath.Join(rStr, targetDir)
					}
				}
			}
			archivePath = filepath.Clean(archivePath)
			if targetDir == "" {
				targetDir = filepath.Dir(archivePath)
			}
			targetDir = filepath.Clean(targetDir)

			action := strings.ToLower(strings.TrimSpace(params.Action))
			isZip := strings.HasSuffix(archivePath, ".zip")
			isTarGz := strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz")

			if !isZip && !isTarGz {
				return "", fmt.Errorf("unsupported archive format %q. Archive must end in .zip or .tar.gz", archivePath)
			}

			switch action {
			case "list":
				if isZip {
					r, err := zip.OpenReader(archivePath)
					if err != nil {
						return "", fmt.Errorf("failed to open zip file: %w", err)
					}
					defer r.Close()

					var entries []map[string]interface{}
					for _, f := range r.File {
						entries = append(entries, map[string]interface{}{
							"name":           f.Name,
							"size_bytes":     f.UncompressedSize64,
							"is_dir":         f.FileInfo().IsDir(),
							"modified_time":  f.Modified.Format("2006-01-02 15:04:05"),
						})
					}
					outBytes, _ := json.MarshalIndent(map[string]interface{}{
						"archive_path": archivePath,
						"file_count":   len(entries),
						"entries":      entries,
					}, "", "  ")
					return string(outBytes), nil
				} else {
					f, err := os.Open(archivePath)
					if err != nil {
						return "", fmt.Errorf("failed to open tar.gz file: %w", err)
					}
					defer f.Close()

					gzr, err := gzip.NewReader(f)
					if err != nil {
						return "", fmt.Errorf("failed to create gzip reader: %w", err)
					}
					defer gzr.Close()

					tr := tar.NewReader(gzr)
					var entries []map[string]interface{}

					for {
						header, err := tr.Next()
						if err == io.EOF {
							break
						}
						if err != nil {
							return "", fmt.Errorf("tar read error: %w", err)
						}
						entries = append(entries, map[string]interface{}{
							"name":          header.Name,
							"size_bytes":    header.Size,
							"is_dir":        header.Typeflag == tar.TypeDir,
							"modified_time": header.ModTime.Format("2006-01-02 15:04:05"),
						})
					}
					outBytes, _ := json.MarshalIndent(map[string]interface{}{
						"archive_path": archivePath,
						"file_count":   len(entries),
						"entries":      entries,
					}, "", "  ")
					return string(outBytes), nil
				}

			case "extract":
				if err := os.MkdirAll(targetDir, 0755); err != nil {
					return "", fmt.Errorf("failed to create target dir: %w", err)
				}

				extractedCount := 0
				if isZip {
					r, err := zip.OpenReader(archivePath)
					if err != nil {
						return "", fmt.Errorf("failed to open zip file: %w", err)
					}
					defer r.Close()

					for _, f := range r.File {
						fpath := filepath.Join(targetDir, f.Name)
						// Zip Slip security guard
						if !strings.HasPrefix(fpath, filepath.Clean(targetDir)+string(os.PathSeparator)) {
							return "", fmt.Errorf("illegal file path in zip: %s", f.Name)
						}

						if f.FileInfo().IsDir() {
							_ = os.MkdirAll(fpath, os.ModePerm)
							continue
						}

						if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
							return "", err
						}

						outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
						if err != nil {
							return "", err
						}

						rc, err := f.Open()
						if err != nil {
							outFile.Close()
							return "", err
						}

						_, err = io.Copy(outFile, rc)
						outFile.Close()
						rc.Close()
						if err != nil {
							return "", err
						}
						extractedCount++
					}
				} else {
					f, err := os.Open(archivePath)
					if err != nil {
						return "", fmt.Errorf("failed to open tar.gz: %w", err)
					}
					defer f.Close()

					gzr, err := gzip.NewReader(f)
					if err != nil {
						return "", err
					}
					defer gzr.Close()

					tr := tar.NewReader(gzr)
					for {
						header, err := tr.Next()
						if err == io.EOF {
							break
						}
						if err != nil {
							return "", err
						}

						target := filepath.Join(targetDir, header.Name)
						if !strings.HasPrefix(target, filepath.Clean(targetDir)+string(os.PathSeparator)) {
							return "", fmt.Errorf("illegal file path in tar: %s", header.Name)
						}

						switch header.Typeflag {
						case tar.TypeDir:
							_ = os.MkdirAll(target, 0755)
						case tar.TypeReg:
							_ = os.MkdirAll(filepath.Dir(target), 0755)
							outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, header.FileInfo().Mode())
							if err != nil {
								return "", err
							}
							_, err = io.Copy(outFile, tr)
							outFile.Close()
							if err != nil {
								return "", err
							}
							extractedCount++
						}
					}
				}
				return fmt.Sprintf("Successfully extracted %d files into %s", extractedCount, targetDir), nil

			case "create":
				if len(params.Files) == 0 {
					return "", fmt.Errorf("files array is required for create action")
				}
				_ = os.MkdirAll(filepath.Dir(archivePath), 0755)

				if isZip {
					newZipFile, err := os.Create(archivePath)
					if err != nil {
						return "", fmt.Errorf("failed to create zip file: %w", err)
					}
					defer newZipFile.Close()

					zipWriter := zip.NewWriter(newZipFile)
					defer zipWriter.Close()

					addedCount := 0
					for _, srcFile := range params.Files {
						fullPath := srcFile
						if !filepath.IsAbs(fullPath) {
							fullPath = filepath.Join(targetDir, srcFile)
						}
						info, err := os.Stat(fullPath)
						if err != nil {
							continue
						}

						if !info.IsDir() {
							w, err := zipWriter.Create(filepath.Base(fullPath))
							if err != nil {
								continue
							}
							fData, err := os.Open(fullPath)
							if err != nil {
								continue
							}
							_, _ = io.Copy(w, fData)
							fData.Close()
							addedCount++
						}
					}
					return fmt.Sprintf("Successfully created zip archive %s with %d files", archivePath, addedCount), nil
				} else {
					mw, err := os.Create(archivePath)
					if err != nil {
						return "", err
					}
					defer mw.Close()

					gzw := gzip.NewWriter(mw)
					defer gzw.Close()

					tw := tar.NewWriter(gzw)
					defer tw.Close()

					addedCount := 0
					for _, srcFile := range params.Files {
						fullPath := srcFile
						if !filepath.IsAbs(fullPath) {
							fullPath = filepath.Join(targetDir, srcFile)
						}
						info, err := os.Stat(fullPath)
						if err != nil || info.IsDir() {
							continue
						}

						header, err := tar.FileInfoHeader(info, info.Name())
						if err != nil {
							continue
						}
						header.Name = filepath.Base(fullPath)

						if err := tw.WriteHeader(header); err != nil {
							continue
						}

						file, err := os.Open(fullPath)
						if err != nil {
							continue
						}
						_, _ = io.Copy(tw, file)
						file.Close()
						addedCount++
					}
					return fmt.Sprintf("Successfully created tar.gz archive %s with %d files", archivePath, addedCount), nil
				}

			default:
				return "", fmt.Errorf("unsupported action %q. Use list, extract, create", action)
			}
		},
	}
}
