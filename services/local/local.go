package local

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/raitonoberu/sptlrx/lyrics"
)

var replacer = strings.NewReplacer(
	"_", " ", "-", " ", ",", "", ".", "", "!", "", "?", "",
	"(", "", ")", "", "[", "", "]", "", "'", "", `"`, "",
)
var lrcTimeLine = regexp.MustCompile(`^\[\d{2}:\d{2}\.\d{2}\]`)

type file struct {
	Path       string
	NameParts  []string
	TitleParts []string // 新增，用于ti标签匹配
}

func New(folder string) (*Client, error) {
	index, err := createIndex(folder)
	if err != nil {
		return nil, err
	}
	return &Client{index: index}, nil
}

// Client implements lyrics.Provider
type Client struct {
	index []*file
}

func (c *Client) Lyrics(id, query string) ([]lyrics.Line, error) {
	f := c.findFile(query)
	if f == nil {
		return nil, nil
	}

	reader, err := os.Open(f.Path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return parseLrcFile(reader), nil
}

func (c *Client) findFile(query string) *file {
	parts := splitString(query)
	var best *file
	var maxScore int

	for _, f := range c.index {
		score := 0

		// 先匹配 ti 标签
		for _, part := range parts {
			for _, titlePart := range f.TitleParts {
				if strings.Contains(titlePart, part) {
					score++
					break
				}
			}
		}

		// 如果 ti 匹配不到，再用文件名兜底
		if score == 0 {
			for _, part := range parts {
				for _, namePart := range f.NameParts {
					if strings.Contains(namePart, part) {
						score++
						break
					}
				}
			}
		}

		if score > maxScore {
			maxScore = score
			best = f
			if score >= len(parts) {
				break // 完全匹配，提前返回
			}
		}
	}

	return best
}

func createIndex(folder string) ([]*file, error) {
	if strings.HasPrefix(folder, "~/") {
		dirname, _ := os.UserHomeDir()
		folder = filepath.Join(dirname, folder[2:])
	}

	index := []*file{}
	err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if d == nil {
			return fmt.Errorf("invalid path: %s", path)
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".lrc") {
			return nil
		}

		name := strings.TrimSuffix(d.Name(), ".lrc")
		nameParts := splitString(name)

		titleParts := []string{}
		// 读取文件前几行，找 [ti:...] 标签
		fhandle, err := os.Open(path)
		if err == nil {
			scanner := bufio.NewScanner(fhandle)
			for i := 0; i < 10 && scanner.Scan(); i++ {
				line := scanner.Text()
				if strings.HasPrefix(line, "[ti:") && strings.HasSuffix(line, "]") {
					ti := line[4 : len(line)-1]
					titleParts = splitString(ti)
					break
				}
			}
			fhandle.Close()
		}

		index = append(index, &file{
			Path:       path,
			NameParts:  nameParts,
			TitleParts: titleParts,
		})
		return nil
	})

	return index, err
}

func splitString(s string) []string {
	s = strings.ToLower(s)
	s = replacer.Replace(s)
	return strings.Fields(s)
}

func parseLrcFile(reader io.Reader) []lyrics.Line {
	result := []lyrics.Line{}
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()

		// 只接受真正的时间戳行
		if !lrcTimeLine.MatchString(line) {
			continue
		}

		l := parseLrcLine(line)

		// 过滤只有时间、没有文字的行
		if strings.TrimSpace(l.Words) == "" {
			continue
		}

		result = append(result, l)
	}

	return result
}

func parseLrcLine(line string) lyrics.Line {
	// [00:00.00]text -> {"time": 0, "words": "text"}
	h, _ := strconv.Atoi(line[1:3])
	m, _ := strconv.Atoi(line[4:6])
	s, _ := strconv.Atoi(line[7:9])

	return lyrics.Line{
		Time:  h*60*1000 + m*1000 + s*10,
		Words: line[10:],
	}
}
