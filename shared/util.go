package shared

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/flosch/pongo2/v4"
	"golang.org/x/sys/unix"
	yaml "gopkg.in/yaml.v2"
)

const (
	ContextKeyEnviron = ContextKey("environ")
	ContextKeyStderr  = ContextKey("stderr")
	EnvRootUUID       = "LXD_IMAGEBUILDER_ROOT_UUID"
)

// EnvVariable represents a environment variable.
type EnvVariable struct {
	Value string
	Set   bool
}

// Environment represents a set of environment variables.
type Environment map[string]EnvVariable

// ContextKey type.
type ContextKey string

// WriteFunc type.
type WriteFunc func([]byte) (int, error)

// Write implements io.Writer interface.
func (w WriteFunc) Write(b []byte) (int, error) {
	return w(b)
}

// CaseInsensitive returns case insensive pattern used by filepath.Glob or filepath.Match.
func CaseInsensitive(s string) (pattern string) {
	s1 := strings.ToLower(s)
	s2 := strings.ToUpper(s)
	for i := range s {
		a := s1[i : i+1]
		b := s2[i : i+1]
		if a != b {
			pattern += "[" + a + b + "]"
		} else if strings.Contains("?*[]/", a) {
			pattern += a
		} else {
			pattern += "\\" + a
		}
	}
	return
}

// FindFirstMatch find the first matched file case insensitive.
func FindFirstMatch(dir string, elem ...string) (found string, err error) {
	matches, err := FindAllMatches(dir, elem...)
	if err != nil {
		return
	}

	found = matches[0]
	return
}

// FindAllMatches find all the matched files case insensitive.
func FindAllMatches(dir string, elem ...string) (matches []string, err error) {
	names := []string{dir}
	for _, name := range elem {
		names = append(names, CaseInsensitive(name))
	}

	pattern := filepath.Join(names...)
	matches, err = filepath.Glob(pattern)
	if err != nil {
		return
	}

	if len(matches) == 0 {
		err = fmt.Errorf("No match found %s", pattern)
		return
	}

	return
}

// Copy copies a file.
func Copy(src, dest string) error {
	var err error

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("Failed to open file %q: %w", src, err)
	}

	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("Failed to create file %q: %w", dest, err)
	}

	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("Failed to copy file: %w", err)
	}

	return destFile.Sync()
}

// RunCommand runs a command. Stdout is written to the given io.Writer. If nil, it's written to the real stdout. Stderr is always written to the real stderr.
func RunCommand(ctx context.Context, stdin io.Reader, stdout io.Writer, name string, arg ...string) error {
	cmd := exec.CommandContext(ctx, name, arg...)
	env, ok := ctx.Value(ContextKeyEnviron).([]string)
	if ok && len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	if stdin != nil {
		cmd.Stdin = stdin
	}

	if stdout != nil {
		cmd.Stdout = stdout
	} else {
		cmd.Stdout = os.Stdout
	}

	stderr, ok := ctx.Value(ContextKeyStderr).(io.Writer)
	if ok && stderr != nil {
		cmd.Stderr = stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}

// RunScript runs a script hereby setting the SHELL and PATH env variables,
// and redirecting the process's stdout and stderr to the real stdout and stderr
// respectively.
func RunScript(ctx context.Context, content string) error {
	fd, err := unix.MemfdCreate("tmp", 0)
	if err != nil {
		return fmt.Errorf("Failed to create memfd: %w", err)
	}

	defer unix.Close(fd)

	_, err = unix.Write(int(fd), []byte(content))
	if err != nil {
		return fmt.Errorf("Failed to write to memfd: %w", err)
	}

	fdPath := fmt.Sprintf("/proc/self/fd/%d", fd)

	return RunCommand(ctx, nil, nil, fdPath)
}

// Pack creates an uncompressed tarball.
func Pack(ctx context.Context, filename, compression, path string, args ...string) (string, error) {
	err := RunCommand(ctx, nil, nil, "tar", append([]string{"--xattrs", "-cf", filename, "-C", path, "--sort=name"}, args...)...)
	if err != nil {
		// Clean up incomplete tarball
		os.Remove(filename)
		return "", fmt.Errorf("Failed to create tarball: %w", err)
	}

	return compressTarball(ctx, filename, compression)
}

// PackUpdate updates an existing tarball.
func PackUpdate(ctx context.Context, filename, compression, path string, args ...string) (string, error) {
	err := RunCommand(ctx, nil, nil, "tar", append([]string{"--xattrs", "-uf", filename, "-C", path, "--sort=name"}, args...)...)
	if err != nil {
		return "", fmt.Errorf("Failed to update tarball: %w", err)
	}

	return compressTarball(ctx, filename, compression)
}

// compressTarball compresses a tarball, or not.
func compressTarball(ctx context.Context, filename, compression string) (string, error) {
	fileExtension := ""

	args := []string{"-f", filename}

	compression, level, err := ParseCompression(compression)
	if err != nil {
		return "", fmt.Errorf("Failed to parse compression level: %w", err)
	}

	if level != nil {
		if compression == "zstd" && *level > 19 {
			args = append(args, "--ultra")
		}

		args = append(args, "-"+strconv.Itoa(*level))
	}

	// If supported, use as many threads as possible.
	if slices.Contains([]string{"zstd", "xz", "lzma"}, compression) {
		args = append(args, "--threads=0")
	}

	switch compression {
	case "lzop", "zstd":
		// Remove the uncompressed file as the compress fails to do so.
		defer os.Remove(filename)
		fallthrough
	case "bzip2", "xz", "lzip", "lzma", "gzip":
		err := RunCommand(ctx, nil, nil, compression, args...)
		if err != nil {
			return "", fmt.Errorf("Failed to compress tarball %q: %w", filename, err)
		}
	}

	switch compression {
	case "lzop":
		fileExtension = "lzo"
	case "zstd":
		fileExtension = "zst"
	case "bzip2":
		fileExtension = "bz2"
	case "xz":
		fileExtension = "xz"
	case "lzip":
		fileExtension = "lz"
	case "lzma":
		fileExtension = "lzma"
	case "gzip":
		fileExtension = "gz"
	}

	if fileExtension == "" {
		return filename, nil
	}

	return fmt.Sprintf("%s.%s", filename, fileExtension), nil
}

// GetExpiryDate returns an expiry date based on the creationDate and format.
func GetExpiryDate(creationDate time.Time, format string) time.Time {
	regex := regexp.MustCompile(`(?:(\d+)(s|m|h|d|w))*`)
	expiryDate := creationDate

	for _, match := range regex.FindAllStringSubmatch(format, -1) {
		// Ignore empty matches
		if match[0] == "" {
			continue
		}

		var duration time.Duration

		switch match[2] {
		case "s":
			duration = time.Second
		case "m":
			duration = time.Minute
		case "h":
			duration = time.Hour
		case "d":
			duration = 24 * time.Hour
		case "w":
			duration = 7 * 24 * time.Hour
		}

		// Ignore any error since it will be an integer.
		value, _ := strconv.Atoi(match[1])
		expiryDate = expiryDate.Add(time.Duration(value) * duration)
	}

	return expiryDate
}

// RenderTemplate renders a pongo2 template.
func RenderTemplate(template string, iface any) (string, error) {
	// Serialize interface
	data, err := yaml.Marshal(iface)
	if err != nil {
		return "", err
	}

	// Decode document and write it to a pongo2 Context
	var ctx pongo2.Context

	err = yaml.Unmarshal(data, &ctx)
	if err != nil {
		return "", fmt.Errorf("Failed unmarshalling data: %w", err)
	}

	// Load template from string
	tpl, err := pongo2.FromString("{% autoescape off %}" + template + "{% endautoescape %}")
	if err != nil {
		return "", err
	}

	// Get rendered template
	ret, err := tpl.Execute(ctx)
	if err != nil {
		return ret, err
	}

	// Looks like we're nesting templates so run pongo again
	if strings.Contains(ret, "{{") || strings.Contains(ret, "{%") {
		return RenderTemplate(ret, iface)
	}

	return ret, err
}

// SetEnvVariables sets the provided environment variables and returns the
// old ones.
func SetEnvVariables(env Environment) Environment {
	oldEnv := Environment{}

	for k, v := range env {
		// Check whether the env variables are set at the moment
		oldVal, set := os.LookupEnv(k)

		// Store old env variables
		oldEnv[k] = EnvVariable{
			Value: oldVal,
			Set:   set,
		}

		if v.Set {
			os.Setenv(k, v.Value)
		} else {
			os.Unsetenv(k)
		}
	}

	return oldEnv
}

// RsyncLocal copies src to dest using rsync.
func RsyncLocal(ctx context.Context, src string, dest string) error {
	err := RunCommand(ctx, nil, nil, "rsync", "-aHASX", "--devices", src, dest)
	if err != nil {
		return fmt.Errorf("Failed to copy %q to %q: %w", src, dest, err)
	}

	return nil
}

// Retry retries a function up to <attempts> times. This is especially useful for networking.
func Retry(f func() error, attempts uint) error {
	var err error

	for i := uint(0); i < attempts; i++ {
		err = f()
		// Stop retrying if the call succeeded or if the context has been cancelled.
		if err == nil || errors.Is(err, context.Canceled) {
			break
		}

		time.Sleep(time.Second)
	}

	return err
}

// ParseCompression extracts the compression method and level (if any) from the
// compression flag.
func ParseCompression(compression string) (string, *int, error) {
	levelRegex := regexp.MustCompile(`^([\w]+)-(\d{1,2})$`)
	match := levelRegex.FindStringSubmatch(compression)
	if match != nil {
		compression = match[1]
		level, err := strconv.Atoi(match[2])
		if err != nil {
			return "", nil, err
		}

		switch compression {
		case "zstd":
			if 1 <= level && level <= 22 {
				return compression, &level, nil
			}

		case "bzip2", "gzip", "lzo", "lzop":
			// The standalone tool is named lzop, but mksquashfs
			// accepts only lzo. For convenience, accept both.
			if compression == "lzo" {
				compression = "lzop"
			}

			if 1 <= level && level <= 9 {
				return compression, &level, nil
			}

		case "lzip", "lzma", "xz":
			if 0 <= level && level <= 9 {
				return compression, &level, nil
			}

		default:
			return "", nil, fmt.Errorf("Compression method %q does not support specifying levels", compression)
		}

		return "", nil, fmt.Errorf("Invalid compression level %q for method %q", level, compression)
	}

	if compression == "lzo" {
		compression = "lzop"
	}

	return compression, nil, nil
}

// ParseSquashfsCompression extracts the compression method and level (if any)
// from the compression flag for use with mksquashfs.
func ParseSquashfsCompression(compression string) (string, *int, error) {
	levelRegex := regexp.MustCompile(`^([\w]+)-(\d{1,2})$`)
	match := levelRegex.FindStringSubmatch(compression)
	if match != nil {
		compression = match[1]
		level, err := strconv.Atoi(match[2])
		if err != nil {
			return "", nil, err
		}

		switch compression {
		case "zstd":
			if 1 <= level && level <= 22 {
				return compression, &level, nil
			}

		case "gzip", "lzo", "lzop":
			// mkskquashfs accepts only lzo, but the standalone
			// tool is named lzop. For convenience, accept both.
			if compression == "lzop" {
				compression = "lzo"
			}

			if 1 <= level && level <= 9 {
				return compression, &level, nil
			}

		default:
			return "", nil, fmt.Errorf("Squashfs compression method %q does not support specifying levels", compression)
		}

		return "", nil, fmt.Errorf("Invalid squashfs compression level %q for method %q", level, compression)
	}

	if compression == "lzop" {
		compression = "lzo"
	}

	if slices.Contains([]string{"gzip", "lzo", "lz4", "xz", "zstd", "lzma"}, compression) {
		return compression, nil, nil
	}

	return "", nil, fmt.Errorf("Invalid squashfs compression method %q", compression)
}

// AppendToFile opens an existing file and appends the given content to it.
func AppendToFile(path string, content string) error {
	if content == "" {
		return nil
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return err
	}

	return nil
}

// FileHash calculates the combined hash for the given files using the provided
// hash function.
func FileHash(hash hash.Hash, paths ...string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}

	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}

		defer file.Close()

		_, err = io.Copy(hash, file)
		if err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// GZipFile compresses the file on the source path and writes the compressed
// content to the destination path. If destination path is empty, the source
// file name is used with .gz suffix.
func GZipFile(srcPath string, dstPath string) error {
	if dstPath == "" {
		dstPath = fmt.Sprintf("%s.gz", srcPath)
	}

	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}

	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}

	defer dstFile.Close()

	writer, err := gzip.NewWriterLevel(dstFile, gzip.BestCompression)
	if err != nil {
		return err
	}

	defer writer.Close()

	// Copy the source file content to the gzip writer which
	// writes the compresses content to the destination file.
	_, err = io.Copy(writer, srcFile)
	if err != nil {
		return err
	}

	return nil
}

// ReadGZipFile opens the GZ file on the given path and decompresses it
// decode into an array of bytes.
func ReadGZipFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}

	defer reader.Close()

	buf := &bytes.Buffer{}

	_, err = io.Copy(buf, reader)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ReadYAMLFile opens the YAML file on the given path and tries to decode it into
// the given structure.
func ReadYAMLFile[T any](path string, obj *T) (*T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Error opening file: %w", err)
	}

	defer file.Close()

	err = yaml.NewDecoder(file).Decode(obj)
	if err != nil {
		return nil, fmt.Errorf("Error decoding YAML: %w", err)
	}

	return obj, nil
}

// ReadJSONFile opens the JSON file on the given path and tries to decode it into
// the given structure.
func ReadJSONFile[T any](path string, obj *T) (*T, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Error opening file: %w", err)
	}

	defer file.Close()

	err = json.NewDecoder(file).Decode(obj)
	if err != nil {
		return nil, fmt.Errorf("Error decoding JSON: %w", err)
	}

	return obj, nil
}

// WriteJSONFile encodes the given structure into JSON format and writes it to the
// file on a given path.
func WriteJSONFile(path string, obj any) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("Failed creating file: %w", err)
	}

	defer file.Close()

	encoder := json.NewEncoder(file)
	err = encoder.Encode(obj)
	if err != nil {
		return fmt.Errorf("Error encoding JSON: %w", err)
	}

	return nil
}

// MapKeys returns map keys as a list.
func MapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

// HasSuffix returns true if the key matches any of the given suffixes.
func HasSuffix(key string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}

	return false
}

// VerifyChecksum verifies the checksum of the file using the provided hash function.
func VerifyChecksum(filePath string, checksum string, hashFunc hash.Hash) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	defer file.Close()

	if hashFunc != nil {
		hashFunc.Reset()
	}

	_, err = io.Copy(hashFunc, file)
	if err != nil {
		return err
	}

	result := fmt.Sprintf("%x", hashFunc.Sum(nil))

	if result != checksum {
		return fmt.Errorf("Hash mismatch for %s: %q (actual) != %q (expected)", filePath, result, checksum)
	}

	return nil
}
