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

	"github.com/longbridgeapp/opencc"
	"github.com/raitonoberu/sptlrx/lyrics"
)

var t2sConverter *opencc.OpenCC

var replacer = strings.NewReplacer(
	"_", " ", "-", " ", ",", "", ".", "", "!", "", "?", "",
	"(", "", ")", "", "[", "", "]", "", "'", "", `"`, "",
)
var lrcTimeLine = regexp.MustCompile(`^\[\d{2}:\d{2}\.\d{2}\]`)

type file struct {
	Path            string
	NameParts       []string
	NamePartsSimp   []string // 简体
	TitleParts      []string // 用于ti标签匹配
	TitlePartsSimp  []string // 简体
	ArtistParts     []string // 用于ar标签匹配
	ArtistPartsSimp []string // 简体
}

func New(folder string) (*Client, error) {
	var err error
	t2sConverter, err = opencc.New("t2s")
	if err != nil {
		return nil, err
	}
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
	parts := splitString(query, true)
	partsSimp := toSimplifiedSlice(parts)

	var singerParts, singerPartsSimp, titleParts, titlePartsSimp []string
	if len(parts) > 1 {
		singerParts = []string{parts[0]}
		singerPartsSimp = []string{partsSimp[0]}
		titleParts = parts[1:]
		titlePartsSimp = partsSimp[1:]
	} else {
		titleParts = parts
		titlePartsSimp = partsSimp
	}

	type matchResult struct {
		file        *file
		titleScore  int
		singerScore int
	}

	var results []matchResult

	for _, f := range c.index {
		titleScore := 0
		singerScore := 0

		// 歌名匹配 ti 标签和文件名
		for _, part := range titleParts {
			for _, titlePart := range f.TitleParts {
				if strings.Contains(titlePart, part) {
					titleScore++
					break
				}
			}
			for _, namePart := range f.NameParts {
				if strings.Contains(namePart, part) {
					titleScore++
					break
				}
			}
		}
		for _, part := range titlePartsSimp {
			for _, titlePart := range f.TitlePartsSimp {
				if strings.Contains(titlePart, part) {
					titleScore++
					break
				}
			}
			for _, namePart := range f.NamePartsSimp {
				if strings.Contains(namePart, part) {
					titleScore++
					break
				}
			}
		}

		// 歌手名匹配 ar 标签和文件名
		for _, part := range singerParts {
			for _, artistPart := range f.ArtistParts {
				if strings.Contains(artistPart, part) {
					singerScore++
					break
				}
			}
			for _, namePart := range f.NameParts {
				if strings.Contains(namePart, part) {
					singerScore++
					break
				}
			}
		}
		for _, part := range singerPartsSimp {
			for _, artistPart := range f.ArtistPartsSimp {
				if strings.Contains(artistPart, part) {
					singerScore++
					break
				}
			}
			for _, namePart := range f.NamePartsSimp {
				if strings.Contains(namePart, part) {
					singerScore++
					break
				}
			}
		}

		results = append(results, matchResult{file: f, titleScore: titleScore, singerScore: singerScore})
	}

	// 先按歌名得分排序，歌名得分相同再按歌手得分排序
	var best *file
	maxTitleScore := -1
	maxSingerScore := -1
	for _, r := range results {
		if r.titleScore > maxTitleScore ||
			(r.titleScore == maxTitleScore && r.singerScore > maxSingerScore) {
			best = r.file
			maxTitleScore = r.titleScore
			maxSingerScore = r.singerScore
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
		nameParts := splitString(name, false)
		namePartsSimp := toSimplifiedSlice(nameParts)

		titleParts := []string{}
		titlePartsSimp := []string{}
		artistParts := []string{}
		artistPartsSimp := []string{}
		// 读取文件前几行，找 [ti:...] 和 [ar:...] 标签
		fhandle, err := os.Open(path)
		if err == nil {
			scanner := bufio.NewScanner(fhandle)
			for i := 0; i < 10 && scanner.Scan(); i++ {
				line := scanner.Text()
				if strings.HasPrefix(line, "[ti:") && strings.HasSuffix(line, "]") {
					ti := line[4 : len(line)-1]
					titleParts = splitString(ti, false)
					titlePartsSimp = toSimplifiedSlice(titleParts)
				}
				if strings.HasPrefix(line, "[ar:") && strings.HasSuffix(line, "]") {
					ar := line[4 : len(line)-1]
					artistParts = splitString(ar, false)
					artistPartsSimp = toSimplifiedSlice(artistParts)
				}
			}
			fhandle.Close()
		}

		index = append(index, &file{
			Path:            path,
			NameParts:       nameParts,
			NamePartsSimp:   namePartsSimp,
			TitleParts:      titleParts,
			TitlePartsSimp:  titlePartsSimp,
			ArtistParts:     artistParts,
			ArtistPartsSimp: artistPartsSimp,
		})
		return nil
	})

	return index, err
}

func splitString(s string, isQuery bool) []string {
	s = strings.ToLower(s)
	s = replacer.Replace(s)
	return strings.Fields(s)
}

func toSimplifiedSlice(src []string) []string {
	if t2sConverter == nil {
		return src
	}
	dst := make([]string, 0, len(src))
	for _, s := range src {
		if converted, err := t2sConverter.Convert(s); err == nil {
			dst = append(dst, converted)
		} else {
			dst = append(dst, s)
		}
	}
	return dst
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
