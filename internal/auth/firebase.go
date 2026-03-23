package auth

import (
	"context"
	"fmt"

	firebase "firebase.google.com/go/v4"
	firebaseauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

type FirebaseService struct {
	client *firebaseauth.Client
}

// NewFirebaseService initialises the Firebase Admin SDK.
// Credentials are loaded from GOOGLE_APPLICATION_CREDENTIALS env var (path to service account JSON).
// credentialsFile may be empty — in that case ADC (Application Default Credentials) is used,
// which works on GCP VMs automatically.
func NewFirebaseService(ctx context.Context, projectID, credentialsFile string) (*FirebaseService, error) {
	opts := []option.ClientOption{}
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}

	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID}, opts...)
	if err != nil {
		return nil, fmt.Errorf("init firebase app: %w", err)
	}

	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("init firebase auth client: %w", err)
	}

	return &FirebaseService{client: client}, nil
}

// VerifyIDToken verifies a Firebase ID token and returns the decoded token.
// token.Claims["phone_number"] contains the verified phone number.
func (s *FirebaseService) VerifyIDToken(ctx context.Context, idToken string) (*firebaseauth.Token, error) {
	return s.client.VerifyIDToken(ctx, idToken)
}
