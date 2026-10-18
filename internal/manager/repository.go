package manager

import (
	"context"

	"github.com/stashapp/stash/pkg/group"
	"github.com/stashapp/stash/pkg/image"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/session"
	"github.com/stashapp/stash/pkg/user"
)

type SceneService interface {
	Create(ctx context.Context, input models.CreateSceneInput) (*models.Scene, error)
	AssignFile(ctx context.Context, sceneID int, fileID models.FileID) error
	Merge(ctx context.Context, sourceIDs []int, destinationID int, fileDeleter *scene.FileDeleter, options scene.MergeOptions) error
	Destroy(ctx context.Context, scene *models.Scene, fileDeleter *scene.FileDeleter, deleteGenerated, deleteFile, destroyFileEntry bool) error

	FindByIDs(ctx context.Context, ids []int, load ...scene.LoadRelationshipOption) ([]*models.Scene, error)
	sceneFingerprintGetter
}

type ImageService interface {
	Destroy(ctx context.Context, image *models.Image, fileDeleter *image.FileDeleter, deleteGenerated, deleteFile, destroyFileEntry bool) error
	DestroyZipImages(ctx context.Context, zipFile models.File, fileDeleter *image.FileDeleter, deleteGenerated bool) ([]*models.Image, error)
}

type GalleryService interface {
	AddImages(ctx context.Context, g *models.Gallery, toAdd ...int) error
	RemoveImages(ctx context.Context, g *models.Gallery, toRemove ...int) error

	SetCover(ctx context.Context, g *models.Gallery, coverImageId int) error
	ResetCover(ctx context.Context, g *models.Gallery) error

	Destroy(ctx context.Context, i *models.Gallery, fileDeleter *image.FileDeleter, deleteGenerated, deleteFile, destroyFileEntry bool) ([]*models.Image, error)

	ValidateImageGalleryChange(ctx context.Context, i *models.Image, updateIDs models.UpdateIDs) error

	Updated(ctx context.Context, galleryID int) error
}

type GroupService interface {
	Create(ctx context.Context, input *models.CreateGroupInput) error
	UpdatePartial(ctx context.Context, id int, updatedGroup models.GroupPartial, frontImage group.ImageInput, backImage group.ImageInput) (*models.Group, error)

	AddSubGroups(ctx context.Context, groupID int, subGroups []models.GroupIDDescription, insertIndex *int) error
	RemoveSubGroups(ctx context.Context, groupID int, subGroupIDs []int) error
	ReorderSubGroups(ctx context.Context, groupID int, subGroupIDs []int, insertPointID int, insertAfter bool) error
}

type UserService interface {
	session.Authenticator
	AllUsers(ctx context.Context) ([]*models.User, error)
	GetUser(ctx context.Context, username string) (*models.User, error)
	AuthenticateByAPIKey(ctx context.Context, apiKey string) (*models.User, error)
	AuthenticateSession(ctx context.Context, username string, sessionToken string) (*models.User, error)
	AutoLoginUser(ctx context.Context) (*models.User, string, error)

	CreateUser(ctx context.Context, u models.User, password string) error
	CreateUserFrom(ctx context.Context, u models.User, password string, from user.CopyFrom) error
	UpdateUser(ctx context.Context, username string, updated models.User) error
	ChangePassword(ctx context.Context, username, existingPassword, newPassword string) error
	ChangeUserPassword(ctx context.Context, username string, newPassword string) error
	DeleteUser(ctx context.Context, username string) error

	AllRoles(ctx context.Context) ([]*models.Role, error)
	GetRole(ctx context.Context, id string) (*models.Role, error)
	CreateRole(ctx context.Context, r models.Role) (*models.Role, error)
	UpdateRole(ctx context.Context, r models.Role) error
	DeleteRole(ctx context.Context, id string) error
	RoleUserCounts(ctx context.Context) (map[string]int, error)
}
