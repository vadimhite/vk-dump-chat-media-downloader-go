package main

import (
	"flag"
	"log"

	"vk-dump-downloader/internal/gallery"
)

func main() {
	archivePath := flag.String("archive", "./Archive", "path to Archive directory")
	flag.Parse()

	if err := gallery.Generate(*archivePath); err != nil {
		log.Fatal(err)
	}
	if err := gallery.PatchIndexHTML(*archivePath + "/index.html"); err != nil {
		log.Printf("patch index.html: %v", err)
	}
}
