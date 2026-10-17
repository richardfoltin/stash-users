import * as GQL from "src/core/generated-graphql";

// the visibilities of a scene - the least privileged role that can see it -
// from everyone to admins only
export const sceneVisibilities = [
  GQL.RoleEnum.Read,
  GQL.RoleEnum.Modify,
  GQL.RoleEnum.Admin,
];

// returns the message id of the name of the visibility
export function sceneVisibilityMessageID(visibility: string) {
  return `scene_visibility.${visibility.toLowerCase()}`;
}
