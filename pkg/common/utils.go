// Copyright (c) 2023 AccelByte Inc. All Rights Reserved.
// This is licensed software from AccelByte Inc, for limitations
// and restrictions contact your company contract manager.

package common

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func GetEnvInt(key string, fallback int) int {
	str := GetEnv(key, strconv.Itoa(fallback))
	val, err := strconv.Atoi(str)
	if err != nil {
		return fallback
	}

	return val
}

func GetEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(GetEnv(key, strconv.FormatBool(fallback))))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

// GenerateRandomInt generate a random int that is not determined
func GenerateRandomInt() int {
	source := rand.NewSource(time.Now().UnixNano())
	random := rand.New(source)

	return random.Intn(10000)
}

// MakeTraceID create new traceID
// example: service_1234
func MakeTraceID(identifiers ...string) string {
	builder := strings.Builder{}
	for _, id := range identifiers {
		if id == "" {
			continue
		}
		builder.WriteString(id)
		builder.WriteString("_")
	}
	builder.WriteString(strconv.Itoa(GenerateRandomInt()))
	return builder.String()
}
