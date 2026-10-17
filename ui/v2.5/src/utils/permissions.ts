import * as GQL from "src/core/generated-graphql";

interface IObjectPermissions {
  // message id of the name of the objects
  object: string;
  edit: GQL.Permission;
  delete: GQL.Permission;
  merge?: GQL.Permission;
}

// the permissions of each kind of object, in the order of the role editor
export const objectPermissionRows: IObjectPermissions[] = [
  {
    object: "scenes",
    edit: GQL.Permission.ScenesEdit,
    delete: GQL.Permission.ScenesDelete,
    merge: GQL.Permission.ScenesMerge,
  },
  {
    object: "markers",
    edit: GQL.Permission.MarkersEdit,
    delete: GQL.Permission.MarkersDelete,
  },
  {
    object: "images",
    edit: GQL.Permission.ImagesEdit,
    delete: GQL.Permission.ImagesDelete,
  },
  {
    object: "galleries",
    edit: GQL.Permission.GalleriesEdit,
    delete: GQL.Permission.GalleriesDelete,
  },
  {
    object: "chapters",
    edit: GQL.Permission.ChaptersEdit,
    delete: GQL.Permission.ChaptersDelete,
  },
  {
    object: "performers",
    edit: GQL.Permission.PerformersEdit,
    delete: GQL.Permission.PerformersDelete,
    merge: GQL.Permission.PerformersMerge,
  },
  {
    object: "studios",
    edit: GQL.Permission.StudiosEdit,
    delete: GQL.Permission.StudiosDelete,
  },
  {
    object: "tags",
    edit: GQL.Permission.TagsEdit,
    delete: GQL.Permission.TagsDelete,
    merge: GQL.Permission.TagsMerge,
  },
  {
    object: "groups",
    edit: GQL.Permission.GroupsEdit,
    delete: GQL.Permission.GroupsDelete,
  },
];

// the permissions that are not of one kind of object, in the order of the
// role editor
export const otherPermissions: GQL.Permission[] = [
  GQL.Permission.RecordActivity,
  GQL.Permission.UseScrapers,
  GQL.Permission.SaveFilters,
  GQL.Permission.DeleteSavedFilters,
  GQL.Permission.ManageFiles,
  GQL.Permission.ViewRestrictedScenes,
  GQL.Permission.ViewPrivateScenes,
  GQL.Permission.ChangeSceneVisibility,
  GQL.Permission.RunLibraryTasks,
  GQL.Permission.ManageSharedListSettings,
];

// the permission to create and edit the objects of each filter mode
export const editPermissions: Record<GQL.FilterMode, GQL.Permission> = {
  [GQL.FilterMode.Scenes]: GQL.Permission.ScenesEdit,
  [GQL.FilterMode.SceneMarkers]: GQL.Permission.MarkersEdit,
  [GQL.FilterMode.Images]: GQL.Permission.ImagesEdit,
  [GQL.FilterMode.Galleries]: GQL.Permission.GalleriesEdit,
  [GQL.FilterMode.Performers]: GQL.Permission.PerformersEdit,
  [GQL.FilterMode.Studios]: GQL.Permission.StudiosEdit,
  [GQL.FilterMode.Tags]: GQL.Permission.TagsEdit,
  [GQL.FilterMode.Groups]: GQL.Permission.GroupsEdit,
  [GQL.FilterMode.Movies]: GQL.Permission.GroupsEdit,
};

// returns the message id of the name of the permission
export function permissionMessageID(p: GQL.Permission) {
  return `permissions.names.${p.toLowerCase()}`;
}

// returns the message id of the description of a permission of
// otherPermissions
export function permissionDescriptionMessageID(p: GQL.Permission) {
  return `permissions.descriptions.${p.toLowerCase()}`;
}
