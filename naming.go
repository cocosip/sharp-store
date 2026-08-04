package store

import "context"

type NormalizedName struct {
	Container ContainerKey
	FileID    string
}

type NamingService interface {
	Normalize(
		ctx context.Context,
		config ContainerConfig,
		container ContainerKey,
		fileID string,
	) (NormalizedName, error)
}

type KeyNormalizer interface {
	NormalizeContainer(
		ctx context.Context,
		config ContainerConfig,
		container ContainerKey,
	) (ContainerKey, error)
	NormalizeFile(ctx context.Context, config ContainerConfig, fileID string) (string, error)
}

type NamingServiceSet struct {
	normalizers []KeyNormalizer
}

func NewNamingService(normalizers ...KeyNormalizer) NamingService {
	return &NamingServiceSet{normalizers: append([]KeyNormalizer(nil), normalizers...)}
}

func (s *NamingServiceSet) Normalize(
	ctx context.Context,
	config ContainerConfig,
	container ContainerKey,
	fileID string,
) (NormalizedName, error) {
	for _, normalizer := range s.normalizers {
		var err error
		container, err = normalizer.NormalizeContainer(ctx, config, container)
		if err != nil {
			return NormalizedName{}, err
		}
		fileID, err = normalizer.NormalizeFile(ctx, config, fileID)
		if err != nil {
			return NormalizedName{}, err
		}
	}
	return NormalizedName{Container: container, FileID: fileID}, nil
}

type identityNamingService struct{}

func (identityNamingService) Normalize(
	_ context.Context,
	_ ContainerConfig,
	container ContainerKey,
	fileID string,
) (NormalizedName, error) {
	return NormalizedName{Container: container, FileID: fileID}, nil
}
