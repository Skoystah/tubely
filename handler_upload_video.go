package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

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

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read video", err)
		return
	}
	defer file.Close()

	mediaType, _, _ := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if mediaType != "video/mp4" {
		err = errors.New("Incorrect media type")
		respondWithError(w, http.StatusInternalServerError, err.Error(), err)
		return
	}

	const tempVideoFileName = "tubely-upload.mp4"
	tempVideoFile, err := os.CreateTemp("", tempVideoFileName)
	defer os.Remove(tempVideoFileName)
	defer tempVideoFile.Close()

	_, err = io.Copy(tempVideoFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not save thumb file", err)
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

	tempVideoFile.Seek(0, io.SeekStart)
	s3VideoFileName := fmt.Sprintf("%s.%s", randFileNameBase64, fileExtension)
	s3PutObjectParams := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &s3VideoFileName,
		Body:        tempVideoFile,
		ContentType: &mediaType,
	}
	ctx := context.Background()
	_, err = cfg.s3Client.PutObject(ctx, &s3PutObjectParams)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not upload file", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, s3VideoFileName)
	videoData.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not update video URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoData)

}
