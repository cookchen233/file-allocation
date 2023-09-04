package main

import (
	"fmt"
	"github.com/cookchen233/myutil"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir := "./"
	// check required files
	zipFile, excelFile, err := getRequiredFiles(dir)
	if err != nil {
		fmt.Println(err)
		exit()
	}

	unZipDir := dir + filepath.Base(strings.Replace(zipFile, ".zip", "", 1))
	fileAllocation := FileAllocation{
		ZipFile:   zipFile,
		ExcelFile: excelFile,
		SrcDir:    unZipDir,
		DstDir:    unZipDir + "-分配",
	}

	// read the configuration file
	fmt.Printf("识别到配置文件: %v\n", excelFile)
	groups, totalNumber, err := fileAllocation.getGroups(excelFile)
	if err != nil {
		fmt.Printf("读取配置文件发生错误: %v\n", err)
		exit()
	}
	fmt.Printf("人数: %d, 总量: %d\n", len(groups), totalNumber)

	os.RemoveAll(fileAllocation.SrcDir)
	os.RemoveAll(fileAllocation.DstDir)

	// uncompress the zip file
	fmt.Printf("识别到压缩包: %v\n正在解包...\n", zipFile)
	_, err = myutil.UnCompress(unZipDir, zipFile)
	if err != nil {
		fmt.Printf("解包发生错误: %v\n", err)
		exit()
	}

	// scan directory
	files, _ := os.ReadDir(unZipDir)
	if len(files) < totalNumber {
		fmt.Printf("处理失败, 压缩包内文件数量 (%d) 小于配置表总量, 请检查\n", len(files))
		exit()
	}

	// allocate files to groups
	fileAllocation.Allocate(files, groups)
	leftFiles, _ := os.ReadDir(unZipDir)
	fmt.Printf("处理完成, 包内文件总量: %d\n已分配: %d, 剩余: %d\n", len(files), totalNumber, len(leftFiles))
	exit()
}
