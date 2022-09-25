package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/mediocregopher/radix/v4"
	log "github.com/sirupsen/logrus"

	"github.com/pilinux/gorest/config"
	"github.com/pilinux/gorest/database"
	"github.com/pilinux/gorest/database/model"
	"github.com/pilinux/gorest/lib"
	"github.com/pilinux/gorest/service"
)

// VerifyEmail handles jobs for controller.VerifyEmail
func VerifyEmail(payload model.AuthPayload) (httpResponse model.HTTPResponse, httpStatusCode int) {
	data := struct {
		key   string
		value string
	}{}
	data.key = model.EmailVerificationKeyPrefix + payload.VerificationCode

	// get redis client
	client := *database.GetRedis()
	rConnTTL := config.GetConfig().Database.REDIS.Conn.ConnTTL
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(rConnTTL)*time.Second)
	defer cancel()

	// is key available in redis
	result := 0
	if err := client.Do(ctx, radix.FlatCmd(&result, "EXISTS", data.key)); err != nil {
		log.WithError(err).Error("error code: 1061")
		httpResponse.Result = "internal server error"
		httpStatusCode = http.StatusInternalServerError
		return
	}

	if result == 0 {
		httpResponse.Result = "wrong/expired verification code"
		httpStatusCode = http.StatusUnauthorized
		return
	}

	// find key in redis
	if err := client.Do(ctx, radix.FlatCmd(&data.value, "GET", data.key)); err != nil {
		log.WithError(err).Error("error code: 1062")
		httpResponse.Result = "internal server error"
		httpStatusCode = http.StatusInternalServerError
		return
	}

	// delete key from redis
	result = 0
	if err := client.Do(ctx, radix.FlatCmd(&result, "DEL", data.key)); err != nil {
		log.WithError(err).Error("error code: 1063")
	}
	if result == 0 {
		err := errors.New("failed to delete recovery key from redis")
		log.WithError(err).Error("error code: 1064")
	}

	// update verification status in database
	db := database.GetDB()
	auth := model.Auth{}

	if err := db.Where("email = ?", data.value).First(&auth).Error; err != nil {
		httpResponse.Result = "unknown user"
		httpStatusCode = http.StatusUnauthorized
		return
	}

	if auth.VerifyEmail == model.EmailVerified {
		httpResponse.Result = "email already verified"
		httpStatusCode = http.StatusOK
		return
	}

	auth.VerifyEmail = model.EmailVerified
	auth.UpdatedAt = time.Now().Local()

	tx := db.Begin()
	if err := tx.Save(&auth).Error; err != nil {
		tx.Rollback()
		log.WithError(err).Error("error code: 1065")
		httpResponse.Result = "internal server error"
		httpStatusCode = http.StatusInternalServerError
		return
	}
	tx.Commit()

	httpResponse.Result = "email successfully verified"
	httpStatusCode = http.StatusOK
	return
}

// CreateVerificationEmail handles jobs for controller.CreateVerificationEmail
func CreateVerificationEmail(payload model.AuthPayload) (httpResponse model.HTTPResponse, httpStatusCode int) {
	payload.Email = strings.TrimSpace(payload.Email)
	if !lib.ValidateEmail(payload.Email) {
		httpResponse.Result = "wrong email address"
		httpStatusCode = http.StatusBadRequest
		return
	}

	v, err := service.GetUserByEmail(payload.Email)
	if err != nil {
		httpResponse.Result = "user not found"
		httpStatusCode = http.StatusNotFound
		return
	}

	// is email already verified
	if v.VerifyEmail == model.EmailVerified {
		httpResponse.Result = "email already verified"
		httpStatusCode = http.StatusOK
		return
	}

	// verify password
	verifyPass, err := argon2id.ComparePasswordAndHash(payload.Password, v.Password)
	if err != nil {
		log.WithError(err).Error("error code: 1071")
		httpResponse.Result = "internal server error"
		httpStatusCode = http.StatusInternalServerError
		return
	}
	if !verifyPass {
		httpResponse.Result = "wrong credentials"
		httpStatusCode = http.StatusUnauthorized
		return
	}

	// issue new verification code
	if !service.SendEmail(v.Email, model.EmailTypeVerification) {
		httpResponse.Result = "failed to send verification email"
		httpStatusCode = http.StatusServiceUnavailable
		return
	}

	httpResponse.Result = "sent verification email"
	httpStatusCode = http.StatusOK
	return
}
