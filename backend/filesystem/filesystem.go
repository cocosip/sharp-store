package filesystem

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	store "github.com/cocosip/sharp-store"
	"github.com/cocosip/sharp-store/backend/internal/fileutil"
)

const (
	Name       = "filesystem"
	RootKey    = "root"
	BaseURLKey = "base_url"
)

type Backend struct{}

func New() store.Backend {
	return &Backend{}
}

func (*Backend) Name() string {
	return Name
}

func (*Backend) ConfigOptions() []store.ConfigOption {
	return []store.ConfigOption{
		{
			Name:        RootKey,
			Type:        "string",
			Required:    true,
			Example:     "D:/sharp-store",
			Description: "Root directory for stored objects.",
		},
		{
			Name:        BaseURLKey,
			Type:        "string",
			Example:     "https://files.example.test",
			Description: "Optional public base URL for object access URLs.",
		},
	}
}

func (*Backend) ValidateConfig(ctx context.Context, values map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if values[RootKey] == "" {
		return errors.New("root is required")
	}
	return nil
}

func (b *Backend) Save(ctx context.Context, request store.SaveRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if request.Body == nil {
		return "", errors.New("file body is required")
	}
	path, err := b.path(request.FileRequest)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	var originalMode *os.FileMode
	if request.Overwrite {
		info, err := os.Stat(path)
		if err == nil {
			mode := info.Mode().Perm()
			originalMode = &mode
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	temporary := filepath.Join(filepath.Dir(path), ".sharp-store-"+rand.Text())
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(temporary)
	}()
	if originalMode != nil {
		if err := file.Chmod(*originalMode); err != nil {
			return "", err
		}
	}

	if _, err := io.Copy(file, request.Body); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Publish only complete files. Link provides atomic create-if-absent across
	// backend instances and processes, without exposing a partial destination.
	if request.Overwrite {
		err = os.Rename(temporary, path)
	} else {
		err = os.Link(temporary, path)
	}
	if errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("%w: %s", store.ErrFileExists, request.FileID)
	}
	if err != nil {
		return "", err
	}
	return request.FileID, nil
}

func (b *Backend) Delete(ctx context.Context, request store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := b.path(request)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	// Directories created as parent prefixes are not objects.
	if info.IsDir() {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (b *Backend) Exists(ctx context.Context, request store.FileRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	path, err := b.path(request)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (b *Backend) Download(ctx context.Context, request store.DownloadRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	reader, err := b.GetOrNil(ctx, request.FileRequest)
	if err != nil {
		return false, err
	}
	if reader == nil {
		return false, nil
	}
	defer func() { _ = reader.Close() }()
	return fileutil.WriteFile(request.Destination, reader)
}

func (b *Backend) GetOrNil(ctx context.Context, request store.FileRequest) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := b.path(request)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if info, err := file.Stat(); err == nil && info.IsDir() {
		_ = file.Close()
		return nil, nil
	}
	return file, nil
}

func (b *Backend) AccessURL(ctx context.Context, request store.AccessURLRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if request.CheckExists {
		exists, err := b.Exists(ctx, request.FileRequest)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", store.ErrFileNotFound
		}
	}
	baseURL := request.Config.Values[BaseURLKey]
	if baseURL == "" {
		return "", store.ErrUnsupported
	}
	return url.JoinPath(baseURL, request.Key)
}

func (b *Backend) path(request store.FileRequest) (string, error) {
	root := request.Config.Values[RootKey]
	if root == "" {
		return "", fmt.Errorf("%s configuration is required", RootKey)
	}
	if runtime.GOOS == "windows" {
		if err := checkWindowsKey(request.Key); err != nil {
			return "", err
		}
	}
	for _, segment := range strings.FieldsFunc(request.Key, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return "", fmt.Errorf("invalid object key %q", request.Key)
		}
	}
	key := filepath.Clean(filepath.FromSlash(request.Key))
	if key == "." || key == ".." || filepath.IsAbs(key) || strings.HasPrefix(key, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid object key %q", request.Key)
	}
	path := filepath.Join(root, key)
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("object key escapes configured root: %q", request.Key)
	}
	return path, nil
}

// windowsReservedBaseNames are device names that Windows resolves regardless
// of the directory in the path, including with an extension such as nul.txt.
var windowsReservedBaseNames = map[string]struct{}{}

func init() {
	for _, name := range []string{"CON", "PRN", "AUX", "NUL"} {
		windowsReservedBaseNames[name] = struct{}{}
	}
	for index := 1; index <= 9; index++ {
		windowsReservedBaseNames[fmt.Sprintf("COM%d", index)] = struct{}{}
		windowsReservedBaseNames[fmt.Sprintf("LPT%d", index)] = struct{}{}
	}
}

// checkWindowsKey rejects keys that Windows cannot round-trip as regular
// files: colons create hidden NTFS alternate data streams, reserved device
// names resolve to hardware devices instead of files, and trailing dots or
// spaces are silently stripped from the created name.
func checkWindowsKey(key string) error {
	for _, segment := range strings.FieldsFunc(key, func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.ContainsRune(segment, ':') {
			return fmt.Errorf("object key %q contains ':' which Windows cannot store as a regular file", key)
		}
		base := segment
		if index := strings.IndexByte(base, '.'); index >= 0 {
			base = base[:index]
		}
		if _, reserved := windowsReservedBaseNames[strings.ToUpper(base)]; reserved {
			return fmt.Errorf("object key %q uses a reserved Windows device name", key)
		}
		if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return fmt.Errorf("object key %q has a segment ending in '.' or ' ' which Windows strips from file names", key)
		}
	}
	return nil
}
