package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	dirNameSeparator = " - "
	maxNameBytes     = 255
	forbiddenChars   = `/\<>:"|?*`
	modtimeLayout    = "2006-01-02T15:04:05"
)

var allowedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".heic": true,
	".mp4": true, ".mov": true, ".m4v": true, ".3gp": true, ".avi": true,
}

type file struct {
	path    string
	name    string
	size    int64
	modtime string
}

type fileKey struct {
	name    string
	size    int64
	modtime string
}

type serverFile struct {
	OriginalName      string    `json:"original_name"`
	SizeBytes         int64     `json:"size_bytes"`
	OriginalCreatedAt time.Time `json:"original_created_at"`
}

type uploadData struct {
	Modtime     string `json:"modtime"`
	Description string `json:"description"`
}

func writeMultipart(writer *multipart.Writer, src io.Reader, fileName string, data []byte) error {
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, src); err != nil {
		return err
	}
	if err := writer.WriteField("data", string(data)); err != nil {
		return err
	}
	return writer.Close()
}

func uploadFile(baseURL string, dirID int64, item file) error {
	data, err := json.Marshal(uploadData{Modtime: item.modtime})
	if err != nil {
		return err
	}

	src, err := os.Open(item.path)
	if err != nil {
		return err
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)

	go func() {
		defer src.Close()
		pipeWriter.CloseWithError(writeMultipart(writer, src, item.name, data))
	}()

	url := fmt.Sprintf("%s/api/dirs/%d/files", baseURL, dirID)
	resp, err := http.Post(url, writer.FormDataContentType(), pipeReader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

type dirInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func fetchDirs(baseURL string) (map[string]int64, error) {
	resp, err := http.Get(baseURL + "/api/dirs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var dirs []dirInfo
	if err := json.NewDecoder(resp.Body).Decode(&dirs); err != nil {
		return nil, err
	}

	dirIDs := make(map[string]int64, len(dirs))
	for _, dir := range dirs {
		dirIDs[strings.ToLower(dir.Name)] = dir.ID
	}
	return dirIDs, nil
}

func serverDirName(dirKey string) string {
	return strings.ReplaceAll(dirKey, string(filepath.Separator), dirNameSeparator)
}

func sanitizeDirName(name string) string {
	cleaned := strings.Map(func(char rune) rune {
		if strings.ContainsRune(forbiddenChars, char) || unicode.IsControl(char) {
			return '_'
		}
		return char
	}, name)

	for len(cleaned) > maxNameBytes {
		_, size := utf8.DecodeLastRuneInString(cleaned)
		cleaned = cleaned[:len(cleaned)-size]
	}
	return strings.TrimRight(cleaned, " -")
}

func fetchDirFiles(baseURL string, dirID int64) (map[fileKey]bool, error) {
	resp, err := http.Get(fmt.Sprintf("%s/api/dirs/%d/files", baseURL, dirID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var serverFiles []serverFile
	if err := json.NewDecoder(resp.Body).Decode(&serverFiles); err != nil {
		return nil, err
	}

	existing := make(map[fileKey]bool, len(serverFiles))
	for _, onServer := range serverFiles {
		existing[fileKey{
			name:    onServer.OriginalName,
			size:    onServer.SizeBytes,
			modtime: onServer.OriginalCreatedAt.Format(modtimeLayout),
		}] = true
	}
	return existing, nil
}

func createDir(baseURL, name string) (int64, error) {
	payload, err := json.Marshal(map[string]string{"name": name, "description": ""})
	if err != nil {
		return 0, err
	}

	resp, err := http.Post(baseURL+"/api/dirs", "application/json", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var dir dirInfo
	if err := json.NewDecoder(resp.Body).Decode(&dir); err != nil {
		return 0, err
	}
	return dir.ID, nil
}

func main() {
	serverFlag := flag.String("server", "http://localhost:8080", "адрес сервера FileVault")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: optimport [-server URL] <dir>\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	serverURL := strings.TrimRight(*serverFlag, "/")
	root := flag.Arg(0)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		fmt.Printf("путь к архиву: %v\n", err)
		os.Exit(1)
	}
	rootName := filepath.Base(absRoot)
	fmt.Printf("сервер: %s\nархив: %s\n\n", serverURL, absRoot)

	files := make(map[string][]file)
	var skipped []string

	if err := filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if path != root && strings.HasPrefix(info.Name(), ".") {
				relDir, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				skipped = append(skipped, relDir+string(filepath.Separator))
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		name := info.Name()
		if strings.HasPrefix(name, ".") || !allowedExts[strings.ToLower(filepath.Ext(name))] {
			skipped = append(skipped, relPath)
			return nil
		}

		dirKey := filepath.Join(rootName, filepath.Dir(relPath))

		files[dirKey] = append(files[dirKey], file{
			path:    path,
			name:    name,
			size:    info.Size(),
			modtime: info.ModTime().Format(modtimeLayout),
		})

		return nil
	}); err != nil {
		fmt.Printf("Какая-то ошибка: %v\n", err)
	}

	totalFiles := 0
	for _, dirKey := range slices.Sorted(maps.Keys(files)) {
		dirFiles := files[dirKey]
		fmt.Printf("%s  (%d)\n", dirKey, len(dirFiles))
		for _, dirFile := range dirFiles {
			fmt.Printf("    %s  %s\n", dirFile.name, dirFile.modtime)
		}
		totalFiles += len(dirFiles)
	}
	fmt.Printf("групп: %d, файлов: %d\n", len(files), totalFiles)

	if len(skipped) > 0 {
		fmt.Printf("\nпропущено: %d\n", len(skipped))
		for _, skippedPath := range skipped {
			fmt.Printf("    %s\n", skippedPath)
		}
	}

	dirIDs, err := fetchDirs(serverURL)
	if err != nil {
		fmt.Printf("список папок: %v\n", err)
		return
	}

	uploaded := 0
	alreadyThere := 0
	var failed []string

	for _, dirKey := range slices.Sorted(maps.Keys(files)) {
		dirFiles := files[dirKey]
		var dirID int64
		dirCreated := false

		rawName := serverDirName(dirKey)
		dirName := sanitizeDirName(rawName)
		if dirName != rawName {
			fmt.Printf("\nимя изменено: %q → %q\n", rawName, dirName)
		}

		if existingID, ok := dirIDs[strings.ToLower(dirName)]; ok {
			dirID = existingID
		} else {
			createdID, err := createDir(serverURL, dirName)
			if err != nil {
				fmt.Printf("\n%s: папка не создана: %v\n", dirName, err)
				for _, item := range dirFiles {
					failed = append(failed, fmt.Sprintf("%s: папка не создана: %v", item.path, err))
				}
				continue
			}
			dirIDs[strings.ToLower(dirName)] = createdID
			dirID = createdID
			dirCreated = true
			fmt.Printf("\nсоздана папка %q (id %d)\n", dirName, dirID)
		}

		existing := map[fileKey]bool{}
		if !dirCreated {
			dirFilesOnServer, err := fetchDirFiles(serverURL, dirID)
			if err != nil {
				fmt.Printf("\n%s: список файлов не получен: %v\n", dirKey, err)
				for _, item := range dirFiles {
					failed = append(failed, fmt.Sprintf("%s: список файлов папки не получен: %v", item.path, err))
				}
				continue
			}
			existing = dirFilesOnServer
		}

		fmt.Printf("\nзагрузка %s → id %d:\n", dirKey, dirID)
		for _, item := range dirFiles {
			if existing[fileKey{name: item.name, size: item.size, modtime: item.modtime}] {
				alreadyThere++
				fmt.Printf("    %s: уже есть\n", item.name)
				continue
			}
			if err := uploadFile(serverURL, dirID, item); err != nil {
				fmt.Printf("    %s: %v\n", item.name, err)
				failed = append(failed, fmt.Sprintf("%s: %v", item.path, err))
				continue
			}
			uploaded++
			fmt.Printf("    %s: ok\n", item.name)
		}
	}

	fmt.Printf("\nзагружено: %d, уже было: %d, ошибок: %d\n", uploaded, alreadyThere, len(failed))
	for _, failure := range failed {
		fmt.Printf("    %s\n", failure)
	}
}
