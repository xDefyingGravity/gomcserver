package download

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// DownloadFile downloads a file from the specified URL and saves it to the given output path.
// If expectedSha1 is not empty, it verifies the downloaded file's SHA-1 hash against the expected value.
// Returns an error if the download, file creation, writing, or hash verification fails.
//
// url:          The URL to download the file from.
// output:       The local file path to save the downloaded file.
// expectedSha1: The expected SHA-1 hash of the file (as a hex string). If empty, no check is performed.
func DownloadFile(url string, output string, expectedSha1 string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	out, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("file create failed: %w", err)
	}
	defer func(out *os.File) {
		_ = out.Close()
	}(out)

	hasher := sha1.New()
	multiWriter := io.MultiWriter(out, hasher)

	if _, err := io.Copy(multiWriter, resp.Body); err != nil {
		return fmt.Errorf("file write failed: %w", err)
	}

	actualSha1 := fmt.Sprintf("%x", hasher.Sum(nil))
	if expectedSha1 != "" && actualSha1 != expectedSha1 {
		return fmt.Errorf("sha1 mismatch: got %s, expected %s", actualSha1, expectedSha1)
	}

	return nil
}

// DownloadJSON downloads JSON data from the specified URL and unmarshals it into a value of type T.
// Returns a pointer to the unmarshaled value or an error if the download or unmarshal fails.
//
// T:   The type to unmarshal the JSON into (must be a struct or compatible type).
// url: The URL to download the JSON from.
func DownloadJSON[T any](url string) (*T, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("http get failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	return &result, nil
}