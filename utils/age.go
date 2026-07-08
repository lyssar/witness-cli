package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"filippo.io/age"
)

var (
	ageRecipients  []age.Recipient
	loaderOnce     sync.Once
	recipientError error
)

func loadAgeRecipients(key string) ([]age.Recipient, error) {
	loaderOnce.Do(func() {
		keyFile, err := os.Open(key)
		if err != nil {
			recipientError = fmt.Errorf("could not open key file %w", err)
			return
		}

		defer func() {
			err := keyFile.Close()
			if err != nil && recipientError == nil {
				recipientError = fmt.Errorf("could not close key file %w", err)
			}
		}()

		identities, err := age.ParseIdentities(keyFile)
		if err != nil {
			recipientError = fmt.Errorf("could not parse age file %w", err)
			return
		}

		var recipients []age.Recipient
		for _, identity := range identities {
			switch typedIdentity := identity.(type) {
			case *age.X25519Identity:
				recipients = append(recipients, typedIdentity.Recipient())
			}
		}

		if len(recipients) == 0 {
			recipientError = fmt.Errorf("couldn't extract recipient from age file")
			return
		}

		ageRecipients = recipients
	})
	return ageRecipients, recipientError
}

func EncryptSecret(ciphertext string, key string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" || key == "" {
		return "", nil
	}

	recipients, err := loadAgeRecipients(key)
	if err != nil {
		return "", fmt.Errorf("error while loading recipients %w", err)
	}

	var cipherBuffer bytes.Buffer
	encryptionWriter, err := age.Encrypt(&cipherBuffer, recipients...)

	if err != nil {
		return "", fmt.Errorf("encryption writer error %w", err)
	}

	if _, err := io.WriteString(encryptionWriter, ciphertext); err != nil {
		return "", fmt.Errorf("couldn't write cipher to encryption %w", err)
	}

	if err := encryptionWriter.Close(); err != nil {
		return "", fmt.Errorf("couldn't close buffer %w", err)
	}

	cipher := cipherBuffer.String()

	return base64.StdEncoding.EncodeToString([]byte(cipher)), nil
}
