package backup

import (
	"archive/tar"
	"github.com/klauspost/compress/zstd"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func CreateBackup(src, destParent string) error {
	dest := filepath.Join(destParent, "backup-"+time.Now().Format("20060102-150405")+".tar.zst")

	if err := createBackupTar(src, dest); err != nil {
		return err
	}

	return nil
}

func createBackupTar(src, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	encoder, err := zstd.NewWriter(out)
	if err != nil {
		return err
	}
	defer func(encoder *zstd.Encoder) {
		_ = encoder.Close()
	}(encoder)

	tarWriter := tar.NewWriter(encoder)
	defer func(tarWriter *tar.Writer) {
		_ = tarWriter.Close()
	}(tarWriter)

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		if relPath == "backups" || strings.HasPrefix(relPath, "backups"+string(os.PathSeparator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		header.Format = tar.FormatPAX

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func(file *os.File) {
				_ = file.Close()
			}(file)

			_, err = io.Copy(tarWriter, file)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func RestoreBackup(path string, directory string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	decoder, err := zstd.NewReader(in)
	if err != nil {
		return err
	}
	defer decoder.Close()

	tarReader := tar.NewReader(decoder)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// skip backups folder to not overwrite it
		if header.Name == "backups" || strings.HasPrefix(header.Name, "backups"+string(os.PathSeparator)) {
			continue
		}

		targetPath := filepath.Join(directory, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			_, err = io.Copy(file, tarReader)
			if err != nil {
				return err
			}

			err = file.Close()
			if err != nil {
				return err
			}
		}
	}

	return nil
}
