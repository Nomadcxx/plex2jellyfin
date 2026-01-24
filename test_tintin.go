package main

import (
	"fmt"
	"github.com/Nomadcxx/jellywatch/internal/naming"
)

func main() {
	filename := "The.Adventure.Of.Tintin.2011.1080p.BluRay.AVC.DTS-HD.MA.5.1.x264-PANAM.mkv"
	path := "/mnt/NVME3/Sabnzbd/complete/movies/The.Adventure.Of.Tintin.2011.1080p.BluRay.AVC.DTS-HD.MA.5.1.x264-PANAM/" + filename
	
	fmt.Printf("Filename: %s\n", filename)
	fmt.Printf("IsMovieFilename: %v\n", naming.IsMovieFilename(filename))
	fmt.Printf("IsTVEpisodeFilename: %v\n", naming.IsTVEpisodeFilename(filename))
	fmt.Printf("IsTVEpisodeFromPath: %v\n", naming.IsTVEpisodeFromPath(path))
	fmt.Printf("IsMovieFromPath: %v\n", naming.IsMovieFromPath(path))
	
	if info, err := naming.ParseMovieName(filename); err == nil {
		fmt.Printf("Movie parse: Title=%q Year=%q\n", info.Title, info.Year)
	} else {
		fmt.Printf("Movie parse error: %v\n", err)
	}
	
	if tvInfo, err := naming.ParseTVShowFromPath(path); err == nil {
		fmt.Printf("TV parse: Title=%q Year=%q Season=%d Episode=%d\n", tvInfo.Title, tvInfo.Year, tvInfo.Season, tvInfo.Episode)
	} else {
		fmt.Printf("TV parse error: %v\n", err)
	}
}
