package services

import (
	"io"
	"net/url"
	"path"
	"strings"
	"testing"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/shabatoily/govfs/pkg/drivers/localstorage"
)

func TestVfsResponsesPreserveDirectoryPaths(t *testing.T) {
	fs, err := localstorage.New(&localstorage.Config{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	service := NewVfsService(fs, "/vfs")
	directory, err := service.Mkdir("/보고서 & #+?")
	if err != nil {
		t.Fatal(err)
	}
	file, err := service.Create(path.Join(directory.Path, "file.txt"), strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := service.List(vfs.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("목록 항목 수 = %d", len(list))
	}
	results, err := service.Search("보고서")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("검색 항목 수 = %d", len(results))
	}
	tree, err := service.Tree(vfs.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("트리 자식 수 = %d", len(tree.Children))
	}
	for _, meta := range []types.MetaRes{directory, list[0], results[0], tree.Children[0].Meta, tree.Meta} {
		parsed, err := url.Parse(meta.URL)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Path != "/vfs" || parsed.Query().Get("q") != meta.Path || parsed.Fragment != "" {
			t.Fatalf("디렉터리 URL에 경로가 보존되지 않았습니다: path=%q, url=%q", meta.Path, meta.URL)
		}
	}
	stat, err := service.Stat(file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if file.URL != "/vfs/"+file.ID.String() || stat.URL != file.URL || tree.Children[0].Children[0].Meta.URL != file.URL {
		t.Fatal("파일 접근 URL이 일치하지 않습니다")
	}
	written, err := service.Write(file.ID, strings.NewReader("수정된 내용"))
	if err != nil {
		t.Fatal(err)
	}
	if written.URL != file.URL {
		t.Fatal("파일 수정 후 접근 URL이 변경되었습니다")
	}
	opened, err := service.Read(file.ID)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := io.ReadAll(opened)
	closeErr := opened.Close()
	if readErr != nil || closeErr != nil || string(content) != "수정된 내용" {
		t.Fatalf("파일 내용 = %q, read=%v, close=%v", content, readErr, closeErr)
	}
	empty, err := service.Mkdir("/empty")
	if err != nil {
		t.Fatal(err)
	}
	emptyList, err := service.List(empty.Path)
	if err != nil {
		t.Fatal(err)
	}
	if emptyList == nil || len(emptyList) != 0 {
		t.Fatalf("빈 목록 = %#v", emptyList)
	}
}
