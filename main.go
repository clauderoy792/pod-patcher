package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/cheggaaa/pb/v3"
	"hash/crc32"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

var fileListUrl = "https://raw.githubusercontent.com/GreenDude120/PoD-Launcher/master/files.xml"
var podDir = ""

func main() {
	if len(os.Args) <= 1 {
		log.Fatalln("you need to pass the path to your pod folder ex: ./pod-patcher '/Users/user/.wine/games/Diablo II/Path Of Diablo'")
	}
	podDir = os.Args[1]
	forceUpdate := len(os.Args) > 2 && os.Args[2] == "-force"
	if _, err := os.Stat(podDir); os.IsNotExist(err) {
		log.Fatalln("The path you passed does not exist")
	} else if err = findPodDir(podDir); err != nil {
		log.Fatalln("The path is not a valid POD path ", err)
	}

	// fetch file to deleteOldAndDownload.
	content := fetchFileList()
	var files Filelist

	// we unmarshal our bytes into our struct
	if err := xml.Unmarshal([]byte(content), &files); err != nil {
		log.Fatalln("failed to unmarshal xml: ", err)
	}

	outdatedFiles := []FileInf{}
	//Check files to update
	for _, file := range files.File {
		p := path.Join(podDir, file.Name)
		if _, err := os.Stat(p); err == nil { //file exists
			content, err := ioutil.ReadFile(p)
			if err != nil {
				log.Fatal(err)
			} else if crc := getChecksum(string(content)); forceUpdate || (file.Crc != "" && crc != file.Crc) { //If checksum is different
				outdatedFiles = append(outdatedFiles, file)
			}
		}
	}
	if len(outdatedFiles) == 0 {
		fmt.Println("All files are up to date")
	} else {
		wg := sync.WaitGroup{}
		//temp folder
		downloadDir := path.Join(podDir, "/temp")
		if err := createDirIfNotExist(downloadDir); err != nil {
			log.Fatalln("Failed to create temp download directory ", err)
		}
		fmt.Printf("Will download %v outdated files", len(outdatedFiles))

		//Download every outdated file
		bar := pb.Default.Start(len(outdatedFiles))
		for _, file := range outdatedFiles {
			//Download the https version of the file
			for _, url := range file.Link {
				if strings.Index(url, "https://") == 0 {
					wg.Add(1)
					go downloadAndReplace(file.Name, downloadDir, url, file.Crc, &wg, bar)
					break
				}
			}
		}

		wg.Wait()

		//Delete temp download folder
		os.RemoveAll(downloadDir)
		bar.Finish()
		fmt.Printf("Downloaded %v files sucessfully", len(outdatedFiles))

	}
}

func downloadAndReplace(file, downloadDir, url, checksum string, w *sync.WaitGroup, bar *pb.ProgressBar) {
	filePath, podFile := path.Join(downloadDir, file), path.Join(podDir, file)

	if content, err := downloadFile(filePath, url); err != nil {
		log.Fatalf("failed to download file %v, err: %v\n", filePath, err)
	} else if crc := getChecksum(content); checksum != "" && crc != checksum { //Calculate checksum
		log.Fatalf("Invalid checksum for file %v, expecting %v, got %v", filePath, checksum, crc)
	}

	//Delete existing file
	if _, err := os.Stat(podFile); err == nil {
		if err = os.Remove(podFile); err != nil {
			log.Fatalln("failed to delete file:", filePath, ", err: ", err)
		}
	}

	//Move file to pod dir
	if err := os.Rename(filePath, podFile); err != nil {
		log.Fatalf("failed to rename %v to %v, err: %v\n", filePath, podFile, err)
	}

	w.Done()
	bar.Add(1)
}

func getChecksum(content string) string {
	by := []byte(content)
	check := crc32.ChecksumIEEE(by)
	return fmt.Sprintf("%X", check)
}

func findPodDir(path string) error {
	var err error
	files := []string{"Path of Diablo Launcher.exe", "Diablo II.exe", "Game.exe"}

	err = filepath.Walk(path, func(dir string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		} else if !info.IsDir() {
			for i, file := range files {
				if info.Name() == file {
					//remove element from array
					files = append(files[:i], files[i+1:]...)
					if len(files) == 0 {
						return io.EOF
					}
				}
			}
		}
		return nil
	})

	if len(files) > 0 {
		err = errors.New(fmt.Sprintln("could not find files: ", files))
	}

	if err == nil || err == io.EOF {
		return nil
	} else {
		return err
	}
}

func fetchFileList() string {
	str, err := downloadFile("", fileListUrl)
	if err != nil {
		log.Fatalln("failed to download the list of file ", err)
	}
	return str
}

func createDirIfNotExist(dir string) error {
	var err error
	if _, err = os.Stat(dir); os.IsNotExist(err) {
		err = os.Mkdir(dir, 0755)
	}
	return err
}

func downloadFile(savepath, url string) (string, error) {
	// Get the data
	resp, err := http.Get(url)
	bodyString := ""
	if err != nil {
		return bodyString, err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	bodyString = string(bodyBytes)

	//if savepath is passed, we copy the body content to it.
	if savepath != "" {
		// Create the file
		out, err := os.Create(savepath)
		if err != nil {
			return bodyString, err
		}
		defer out.Close()

		// Write the body to file
		_, err = io.WriteString(out, bodyString)
	}

	return bodyString, err
}

type Filelist struct {
	XMLName xml.Name  `xml:"filelist"`
	Text    string    `xml:",chardata"`
	File    []FileInf `xml:"file"`
}

type FileInf struct {
	Text            string   `xml:",chardata"`
	Name            string   `xml:"name,attr"`
	Crc             string   `xml:"crc,attr"`
	ShowDialog      string   `xml:"showDialog,attr"`
	RestartRequired string   `xml:"restartRequired,attr"`
	Link            []string `xml:"link"`
}
