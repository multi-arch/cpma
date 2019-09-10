package oauth

import (
	"encoding/base64"

	"github.com/fusor/cpma/pkg/io"
	"github.com/fusor/cpma/pkg/transform/configmaps"
	"github.com/fusor/cpma/pkg/transform/secrets"
	configv1 "github.com/openshift/api/config/v1"
	legacyconfigv1 "github.com/openshift/api/legacyconfig/v1"
	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

func buildGitHubIP(serializer *json.Serializer, p IdentityProvider) (*ProviderResources, error) {
	var (
		err                error
		idP                = &configv1.IdentityProvider{}
		providerSecrets    []*corev1.Secret
		providerConfigMaps []*configmaps.ConfigMap
		github             legacyconfigv1.GitHubIdentityProvider
	)

	if _, _, err = serializer.Decode(p.Provider.Raw, nil, &github); err != nil {
		return nil, errors.Wrap(err, "Failed to decode github, see error")
	}

	idP.Type = "GitHub"
	idP.Name = p.Name
	idP.MappingMethod = configv1.MappingMethodType(p.MappingMethod)
	idP.GitHub = &configv1.GitHubIdentityProvider{}
	idP.GitHub.ClientID = github.ClientID
	idP.GitHub.Hostname = github.Hostname
	idP.GitHub.Organizations = github.Organizations
	idP.GitHub.Teams = github.Teams

	if github.CA != "" {
		caConfigmap := configmaps.GenConfigMap("github-configmap", OAuthNamespace, p.CAData)
		idP.GitHub.CA = configv1.ConfigMapNameReference{Name: caConfigmap.Metadata.Name}
		providerConfigMaps = append(providerConfigMaps, caConfigmap)
	}

	secretName := "github-secret"
	idP.GitHub.ClientSecret.Name = secretName
	secretContent, err := io.FetchStringSource(github.ClientSecret)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to fetch client secret for for github, see error")
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(secretContent))
	secret, err := secrets.GenSecret(secretName, encoded, OAuthNamespace, secrets.LiteralSecretType)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to generate client secret for for github, see error")
	}
	providerSecrets = append(providerSecrets, secret)

	return &ProviderResources{
		IDP:        idP,
		Secrets:    providerSecrets,
		ConfigMaps: providerConfigMaps,
	}, nil
}

func validateGithubProvider(serializer *json.Serializer, p IdentityProvider) error {
	var github legacyconfigv1.GitHubIdentityProvider

	if _, _, err := serializer.Decode(p.Provider.Raw, nil, &github); err != nil {
		return errors.Wrap(err, "Failed to decode github, see error")
	}

	if p.Name == "" {
		return errors.New("Name can't be empty")
	}

	if err := validateMappingMethod(p.MappingMethod); err != nil {
		return err
	}

	if github.ClientSecret.KeyFile != "" {
		return errors.New("Usage of encrypted files as secret value is not supported")
	}

	if err := validateClientData(github.ClientID, github.ClientSecret); err != nil {
		return err
	}

	if github.Hostname == "" && (len(github.Organizations) == 0 || len(github.Teams) == 0) {
		return errors.New("GitHub (non enterprise) provider without organizations or teams field is not supported")
	}

	return nil
}
