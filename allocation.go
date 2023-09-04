package main

import (
	"fmt"
	"github.com/cookchen233/myutil"
	"github.com/gookit/goutil"
	"github.com/xuri/excelize/v2"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

type FileAllocation struct {
	ZipFile   string
	ExcelFile string
	SrcDir    string
	DstDir    string
}

type Group struct {
	Name               string
	FileNumber         int
	Password           string
	Dir                string
	completeMoveNumber chan int
	isCompleteMove     chan bool
}

type groupFileInfo struct {
	group *Group
	file  os.DirEntry
}

// getGroups get the group info from Excel file
func (bind *FileAllocation) getGroups(excelFile string) ([]*Group, int, error) {
	f, err := excelize.OpenFile(excelFile)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Println(err)
		}
	}()
	rows, err := f.GetRows("Sheet1")
	var groups []*Group
	totalNumber := 0
	for rowI, row := range rows {
		if rowI == 0 {
			continue
		}
		if len(row) < 1 || row[0] == "" {
			break
		}
		if len(row) < 2 || row[1] == "" || row[1] == "0" {
			continue
		}
		if len(row) < 3 {
			row = append(row, "")
		}
		totalNumber += goutil.Int(row[1])

		name := strings.Trim(row[0], "")
		dstDir := filepath.Join(bind.DstDir, filepath.Base(bind.SrcDir)+"-"+name)
		if !myutil.FileExists(dstDir) {
			os.MkdirAll(dstDir, 0755)
		}
		group := &Group{
			Name:               name,
			FileNumber:         goutil.Int(row[1]),
			Password:           strings.Trim(row[2], ""),
			Dir:                bind.DstDir + "/" + filepath.Base(bind.SrcDir) + "-" + name,
			completeMoveNumber: make(chan int, 1),
			isCompleteMove:     make(chan bool, 1),
		}
		groups = append(groups, group)
	}
	return groups, totalNumber, nil
}

// allocate files to allocation task.
func (bind *FileAllocation) getGroupFileInfos(files []os.DirEntry, groups []*Group) []groupFileInfo {
	var groupFileInfos []groupFileInfo
	groupFileN := 0
	groupI := 0
	for _, file := range files {
		if groupI >= len(groups) {
			break
		}
		group := groups[groupI]
		if groupFileN < group.FileNumber {
			groupFileInfos = append(groupFileInfos, groupFileInfo{file: file, group: groups[groupI]})
			groupFileN++
		} else {
			groupFileN = 0
			groupI++
		}
	}
	return groupFileInfos
}

func (bind *FileAllocation) Allocate(files []os.DirEntry, groups []*Group) {
	// Register complete events for the groups
	for _, group := range groups {
		go func(group *Group) {
			compNum := 0
			for num := range group.completeMoveNumber {
				compNum += num
				if compNum >= group.FileNumber {
					group.isCompleteMove <- true
					close(group.completeMoveNumber)
				}
			}
		}(group)
	}

	// Move files to each of the groups
	groupFileInfos := bind.getGroupFileInfos(files, groups)
	total := len(groupFileInfos)
	ths := runtime.NumCPU() * 2
	step := int(math.Ceil(float64(total) / float64(ths)))
	for i := 0; i < total; i += step {
		go func(i int) {
			for j := 0; j < step; j++ {
				n := i + j
				if n >= total {
					break
				}
				//groupFileInfo := groupFileInfos[n]
				err := bind.moveFile(groupFileInfos[n].file, groupFileInfos[n].group)
				if err != nil {
					fmt.Printf("移动文件时发生错误, file: %v, %v\n", groupFileInfos[n].file.Name(), err)
					exit()
				}
				// notify listeners
				groupFileInfos[n].group.completeMoveNumber <- 1
			}
		}(i)
	}

	// Compress
	groupCompletion := make(chan *Group, 1)
	for _, group := range groups {
		// listen move completion
		go func(group *Group) {
			for isComp := range group.isCompleteMove {
				if isComp {
					err := bind.compress(group)
					if err != nil {
						fmt.Printf("压缩目录时发生错误, path: %v, %v\n", group.Dir, err)
						exit()
					}
					// notify listeners
					groupCompletion <- group
				}
				close(group.isCompleteMove)
			}
		}(group)
	}

	// Print completed messages
	i := 0
	for comp := range groupCompletion {
		i++
		fmt.Printf("%d/%d %v: %v, 分配成功\n", i, len(groups), comp.Name, comp.FileNumber)
		if i >= len(groups) {
			close(groupCompletion)
		}
	}
}

// move a file
func (bind *FileAllocation) moveFile(file os.DirEntry, group *Group) error {
	src := filepath.Join(bind.SrcDir, file.Name())
	dst := filepath.Join(group.Dir, file.Name())
	if !myutil.FileExists(filepath.Dir(dst)) {
		os.MkdirAll(filepath.Dir(dst), 0755)
	}
	_, err := myutil.Copy(src, dst)
	if err != nil {
		return err
	}
	os.RemoveAll(src)
	return nil
}

// the dir to a zip file from group data
func (bind *FileAllocation) compress(group *Group) error {
	output, err := myutil.CompressUsing7z(group.Dir+"/*", filepath.Clean(group.Dir)+".zip", group.Password, "")
	result := string(output)
	if err != nil || strings.Contains(strings.ToLower(result), "error") {
		fmt.Println("compress", group.Dir+"/*", filepath.Clean(group.Dir)+".zip", group.Password)
		return fmt.Errorf("%+v. %+v", result, err)
	}
	os.RemoveAll(group.Dir)
	return nil
}

func exit() {
	sig := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		done <- true
	}()
	<-done
	os.Exit(0)
}

// check required files
func getRequiredFiles(dir string) (string, string, error) {
	files, _ := os.ReadDir(dir)
	var zipFile, excelFile string
	for _, file := range files {
		filename := filepath.Join(dir, file.Name())
		if filepath.Ext(filename) == ".zip" {
			zipFile = filename
		} else if filepath.Ext(filename) == ".xlsx" {
			excelFile = filename
		}
	}
	if excelFile == "" || zipFile == "" {
		return "", "", fmt.Errorf("当前目录必须要有一个 zip 压缩包文件和一个 xlsx 配置文件")
	}
	return zipFile, excelFile, nil
}
