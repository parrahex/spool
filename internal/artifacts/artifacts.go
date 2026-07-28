package artifacts

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Workspace returns a directory that can be mounted into a job container.
// Directories are mounted as-is; single files and archives are copied or
// extracted into a temporary directory that is removed by cleanup.
func Workspace(jobID, sourcePath string) (workspace string, cleanup func(), err error) {
	if sourcePath == "" {
		return "", func() {}, nil
	}

	sourcePath, err = filepath.Abs(sourcePath)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return sourcePath, func() {}, nil
	}

	workspace, err = os.MkdirTemp("", "spool-"+jobID+"-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(workspace) }

	switch strings.ToLower(filepath.Ext(sourcePath)) {
	case ".zip":
		err = extractZIP(sourcePath, workspace)
	case ".gz", ".tgz":
		err = extractTarGZ(sourcePath, workspace)
	default:
		err = copyFile(sourcePath, filepath.Join(workspace, filepath.Base(sourcePath)))
	}
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return workspace, cleanup, nil
}

func extractZIP(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		target, err := safePath(destination, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !file.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("unsupported archive entry %q", file.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		err = writeFile(target, file.Mode(), input)
		input.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGZ(archivePath, destination string) error {
	input, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer input.Close()

	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safePath(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, os.FileMode(header.Mode), reader); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %q", header.Name)
		}
	}
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	return writeFile(destination, info.Mode(), input)
}

func writeFile(path string, mode os.FileMode, input io.Reader) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func safePath(destination, name string) (string, error) {
	target := filepath.Join(destination, name)
	cleanDestination := filepath.Clean(destination) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDestination) {
		return "", fmt.Errorf("archive entry %q escapes destination", name)
	}
	return target, nil
}
