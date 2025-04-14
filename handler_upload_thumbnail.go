package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Could not retrieve video data", err)
		return
	}

	if userID != videoData.UserID {
		err = errors.New("Video does not belong to user")
		respondWithError(w, http.StatusUnauthorized, err.Error(), err)
		return
	}

	const maxMemory = 10 << 20

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read thumbnail", err)
		return
	}
	defer file.Close()

	mediaType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if mediaType != "image/jpg" && mediaType != "image/png" {
		err = errors.New("Incorrect media type")
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	fileExtension := strings.Split(mediaType, "/")[1]
	var randFileName = make([]byte, 32)
	_, err = rand.Read(randFileName)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not create filename", err)
		return
	}
	randFileNameBase64 := base64.RawURLEncoding.EncodeToString(randFileName)

	filePath := filepath.Join(cfg.assetsRoot, (string(randFileNameBase64) + "." + fileExtension))

	thumbFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not create thumb file", err)
		return
	}
	defer thumbFile.Close()

	_, err = io.Copy(thumbFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not save thumb file", err)
		return
	}

	thumbURL := "http://localhost:" + cfg.port + "/" + filePath
	videoData.ThumbnailURL = &thumbURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not update video URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoData)
}
