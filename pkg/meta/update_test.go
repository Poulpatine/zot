package meta_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	notreg "github.com/notaryproject/notation-go/registry"
	godigest "github.com/opencontainers/go-digest"
	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	. "github.com/smartystreets/goconvey/convey"

	zerr "zotregistry.io/zot/errors"
	"zotregistry.io/zot/pkg/extensions/monitoring"
	"zotregistry.io/zot/pkg/log"
	"zotregistry.io/zot/pkg/meta"
	"zotregistry.io/zot/pkg/meta/bolt"
	"zotregistry.io/zot/pkg/meta/repodb"
	bolt_wrapper "zotregistry.io/zot/pkg/meta/repodb/boltdb-wrapper"
	"zotregistry.io/zot/pkg/storage"
	"zotregistry.io/zot/pkg/storage/local"
	"zotregistry.io/zot/pkg/test"
	"zotregistry.io/zot/pkg/test/mocks"
)

var ErrTestError = errors.New("test error")

func TestOnUpdateManifest(t *testing.T) {
	Convey("On UpdateManifest", t, func() {
		rootDir := t.TempDir()
		storeController := storage.StoreController{}
		log := log.NewLogger("debug", "")
		metrics := monitoring.NewMetricsServer(false, log)
		storeController.DefaultStore = local.NewImageStore(rootDir, true, 1*time.Second,
			true, true, log, metrics, nil, nil,
		)

		params := bolt.DBParameters{
			RootDir: rootDir,
		}
		boltDriver, err := bolt.GetBoltDriver(params)
		So(err, ShouldBeNil)

		repoDB, err := bolt_wrapper.NewBoltDBWrapper(boltDriver, log)
		So(err, ShouldBeNil)

		config, layers, manifest, err := test.GetRandomImageComponents(100)
		So(err, ShouldBeNil)

		err = test.WriteImageToFileSystem(
			test.Image{
				Config: config, Manifest: manifest, Layers: layers, Reference: "tag1",
			},
			"repo",
			storeController)
		So(err, ShouldBeNil)

		manifestBlob, err := json.Marshal(manifest)
		So(err, ShouldBeNil)

		digest := godigest.FromBytes(manifestBlob)

		err = meta.OnUpdateManifest("repo", "tag1", "", digest, manifestBlob, storeController, repoDB, log)
		So(err, ShouldBeNil)

		repoMeta, err := repoDB.GetRepoMeta("repo")
		So(err, ShouldBeNil)

		So(repoMeta.Tags, ShouldContainKey, "tag1")
	})

	Convey("metadataSuccessfullySet is false", t, func() {
		rootDir := t.TempDir()
		storeController := storage.StoreController{}
		log := log.NewLogger("debug", "")
		metrics := monitoring.NewMetricsServer(false, log)
		storeController.DefaultStore = local.NewImageStore(rootDir, true, 1*time.Second,
			true, true, log, metrics, nil, nil,
		)

		repoDB := mocks.RepoDBMock{
			SetManifestDataFn: func(manifestDigest godigest.Digest, mm repodb.ManifestData) error {
				return ErrTestError
			},
		}

		err := meta.OnUpdateManifest("repo", "tag1", ispec.MediaTypeImageManifest, "digest",
			[]byte("{}"), storeController, repoDB, log)
		So(err, ShouldNotBeNil)
	})
}

func TestUpdateErrors(t *testing.T) {
	Convey("Update operations", t, func() {
		Convey("On UpdateManifest", func() {
			imageStore := mocks.MockedImageStore{}
			storeController := storage.StoreController{DefaultStore: &imageStore}
			repoDB := mocks.RepoDBMock{}
			log := log.NewLogger("debug", "")

			Convey("CheckIsImageSignature errors", func() {
				badManifestBlob := []byte("bad")

				imageStore.GetImageManifestFn = func(repo, reference string) ([]byte, godigest.Digest, string, error) {
					return []byte{}, "", "", zerr.ErrManifestNotFound
				}

				imageStore.DeleteImageManifestFn = func(repo, reference string, detectCollision bool) error {
					return nil
				}

				err := meta.OnUpdateManifest("repo", "tag1", "digest", "media", badManifestBlob,
					storeController, repoDB, log)
				So(err, ShouldNotBeNil)
			})

			Convey("GetSignatureLayersInfo errors", func() {
				// get notation signature layers info
				badNotationManifestContent := ispec.Manifest{
					Subject: &ispec.Descriptor{
						Digest: "123",
					},
					Config: ispec.Descriptor{MediaType: notreg.ArtifactTypeNotation},
				}

				badNotationManifestBlob, err := json.Marshal(badNotationManifestContent)
				So(err, ShouldBeNil)

				imageStore.GetImageManifestFn = func(repo, reference string) ([]byte, godigest.Digest, string, error) {
					return badNotationManifestBlob, "", "", nil
				}

				err = meta.OnUpdateManifest("repo", "tag1", "", "digest", badNotationManifestBlob,
					storeController, repoDB, log)
				So(err, ShouldNotBeNil)
			})

			Convey("UpdateSignaturesValidity", func() {
				notationManifestContent := ispec.Manifest{
					Subject: &ispec.Descriptor{
						Digest: "123",
					},
					Config: ispec.Descriptor{MediaType: notreg.ArtifactTypeNotation},
					Layers: []ispec.Descriptor{{
						MediaType: ispec.MediaTypeImageLayer,
						Digest:    godigest.FromString("blob digest"),
					}},
				}

				notationManifestBlob, err := json.Marshal(notationManifestContent)
				So(err, ShouldBeNil)

				imageStore.GetImageManifestFn = func(repo, reference string) ([]byte, godigest.Digest, string, error) {
					return notationManifestBlob, "", "", nil
				}

				imageStore.GetBlobContentFn = func(repo string, digest godigest.Digest) ([]byte, error) {
					return []byte{}, nil
				}

				repoDB.UpdateSignaturesValidityFn = func(repo string, manifestDigest godigest.Digest) error {
					return ErrTestError
				}

				err = meta.OnUpdateManifest("repo", "tag1", "", "digest", notationManifestBlob,
					storeController, repoDB, log)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("On DeleteManifest", func() {
			imageStore := mocks.MockedImageStore{}
			storeController := storage.StoreController{DefaultStore: &imageStore}
			repoDB := mocks.RepoDBMock{}
			log := log.NewLogger("debug", "")

			Convey("CheckIsImageSignature errors", func() {
				badManifestBlob := []byte("bad")

				imageStore.GetImageManifestFn = func(repo, reference string) ([]byte, godigest.Digest, string, error) {
					return []byte{}, "", "", zerr.ErrManifestNotFound
				}

				err := meta.OnDeleteManifest("repo", "tag1", "digest", "media", badManifestBlob,
					storeController, repoDB, log)
				So(err, ShouldNotBeNil)
			})

			Convey("DeleteReferrers errors", func() {
				repoDB.DeleteReferrerFn = func(repo string, referredDigest, referrerDigest godigest.Digest) error {
					return ErrTestError
				}

				err := meta.OnDeleteManifest("repo", "tag1", "digest", "media",
					[]byte(`{"subject": {"digest": "dig"}}`),
					storeController, repoDB, log)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("On GetManifest", func() {
			imageStore := mocks.MockedImageStore{}
			storeController := storage.StoreController{DefaultStore: &imageStore}
			repoDB := mocks.RepoDBMock{}
			log := log.NewLogger("debug", "")

			Convey("CheckIsImageSignature errors", func() {
				badManifestBlob := []byte("bad")

				imageStore.GetImageManifestFn = func(repo, reference string) ([]byte, godigest.Digest, string, error) {
					return []byte{}, "", "", zerr.ErrManifestNotFound
				}

				err := meta.OnGetManifest("repo", "tag1", badManifestBlob,
					storeController, repoDB, log)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("SetImageMetaFromInput", func() {
			imageStore := mocks.MockedImageStore{}
			repoDB := mocks.RepoDBMock{}
			log := log.NewLogger("debug", "")

			err := repodb.SetImageMetaFromInput("repo", "ref", ispec.MediaTypeImageManifest, "digest",
				[]byte("BadManifestBlob"), imageStore, repoDB, log)
			So(err, ShouldNotBeNil)

			// reference is digest

			manifestContent := ispec.Manifest{}
			manifestBlob, err := json.Marshal(manifestContent)
			So(err, ShouldBeNil)

			imageStore.GetImageManifestFn = func(repo, reference string) ([]byte, godigest.Digest, string, error) {
				return manifestBlob, "", "", nil
			}
			imageStore.GetBlobContentFn = func(repo string, digest godigest.Digest) ([]byte, error) {
				return []byte("{}"), nil
			}

			err = repodb.SetImageMetaFromInput("repo", string(godigest.FromString("reference")), "", "digest",
				manifestBlob, imageStore, repoDB, log)
			So(err, ShouldBeNil)
		})

		Convey("SetImageMetaFromInput SetData errors", func() {
			imageStore := mocks.MockedImageStore{}
			log := log.NewLogger("debug", "")

			repoDB := mocks.RepoDBMock{
				SetManifestDataFn: func(manifestDigest godigest.Digest, mm repodb.ManifestData) error {
					return ErrTestError
				},
			}
			err := repodb.SetImageMetaFromInput("repo", "ref", ispec.MediaTypeImageManifest, "digest",
				[]byte("{}"), imageStore, repoDB, log)
			So(err, ShouldNotBeNil)
		})

		Convey("SetImageMetaFromInput SetIndexData errors", func() {
			imageStore := mocks.MockedImageStore{}
			log := log.NewLogger("debug", "")

			repoDB := mocks.RepoDBMock{
				SetIndexDataFn: func(digest godigest.Digest, indexData repodb.IndexData) error {
					return ErrTestError
				},
			}
			err := repodb.SetImageMetaFromInput("repo", "ref", ispec.MediaTypeImageIndex, "digest",
				[]byte("{}"), imageStore, repoDB, log)
			So(err, ShouldNotBeNil)
		})

		Convey("SetImageMetaFromInput SetReferrer errors", func() {
			imageStore := mocks.MockedImageStore{
				GetBlobContentFn: func(repo string, digest godigest.Digest) ([]byte, error) {
					return []byte("{}"), nil
				},
			}
			log := log.NewLogger("debug", "")

			repoDB := mocks.RepoDBMock{
				SetReferrerFn: func(repo string, referredDigest godigest.Digest, referrer repodb.ReferrerInfo) error {
					return ErrTestError
				},
			}

			err := repodb.SetImageMetaFromInput("repo", "ref", ispec.MediaTypeImageManifest, "digest",
				[]byte(`{"subject": {"digest": "subjDigest"}}`), imageStore, repoDB, log)
			So(err, ShouldNotBeNil)
		})
	})
}
