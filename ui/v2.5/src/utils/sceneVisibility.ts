import * as GQL from "src/core/generated-graphql";

// the visibilities of a scene - who can see it - from everyone to the users
// who may see private scenes
export const sceneVisibilities = [
  GQL.SceneVisibility.Everyone,
  GQL.SceneVisibility.Restricted,
  GQL.SceneVisibility.Private,
];

// returns the message id of the name of the visibility
export function sceneVisibilityMessageID(visibility: string) {
  return `scene_visibility.${visibility.toLowerCase()}`;
}
