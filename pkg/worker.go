package barertc

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"git.kirsle.net/apps/barertc/pkg/config"
	"git.kirsle.net/apps/barertc/pkg/log"
	"git.kirsle.net/apps/barertc/pkg/models"
)

// StartWorkers initializes background workers, to expire off old DMs and stored images and things.
func (s *Server) StartWorkers() {

	log.Info("Starting BareRTC worker threads")
	go s.workerExpireOldDMs()
	go s.workerExpireOldFiles()

}

func (s *Server) workerExpireOldDMs() {
	for {
		(models.DirectMessage{}).DeleteOldMessages()
		time.Sleep(time.Hour)
	}
}

func (s *Server) workerExpireOldFiles() {
	var (
		reFileFormat = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.(\d+)\.(.+?)\.([a-zA-Z0-9]+?)$`)
		basedir      = "./web/files"
	)

	for {
		cutoff := time.Now().Add(time.Duration(-config.Current.HostImagesExpireDays) * 24 * time.Hour).Format(time.DateOnly)
		log.Info("Worker: Expire old stored image shares (cutoff %s)", cutoff)

		if files, err := os.ReadDir(basedir); err != nil {
			log.Error("Worker: Expire old files: %s", err)
		} else {
			for _, file := range files {
				filename := file.Name()
				if m := reFileFormat.FindStringSubmatch(filename); len(m) > 0 {
					var (
						fileDate         = m[1]
						msgID            = m[2]
						strippedUsername = m[3]
						ext              = m[4]
					)
					log.Info("See file: %s (date:%s msgid:%s user:%s ext:%s)", filename, fileDate, msgID, strippedUsername, ext)
					if fileDate < cutoff {
						log.Error("Worker: Expire old file: %s is older than the cutoff %s, deleting", filename, cutoff)
						os.Remove(fmt.Sprintf("%s/%s", basedir, filename))
					}
				}
			}
		}

		time.Sleep(time.Hour)
	}
}
