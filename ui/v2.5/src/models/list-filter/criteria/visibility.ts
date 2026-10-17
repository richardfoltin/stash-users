import { IntlShape } from "react-intl";
import {
  CriterionModifier,
  Permission,
  SceneVisibility,
  SceneVisibilityCriterionInput,
} from "src/core/generated-graphql";
import {
  sceneVisibilities,
  sceneVisibilityMessageID,
} from "src/utils/sceneVisibility";
import { ModifierCriterionOption, MultiStringCriterion } from "./criterion";

// the old visibility values of saved and URL criteria
const legacyVisibilities: Record<string, SceneVisibility> = {
  READ: SceneVisibility.Everyone,
  MODIFY: SceneVisibility.Restricted,
  ADMIN: SceneVisibility.Private,
};

export const VisibilityCriterionOption = new ModifierCriterionOption({
  messageID: "scene_visibility.visible_to",
  type: "visibility",
  permission: Permission.ChangeSceneVisibility,
  modifierOptions: [CriterionModifier.Includes, CriterionModifier.Excludes],
  defaultModifier: CriterionModifier.Includes,
  options: sceneVisibilities,
  optionMessageID: (o) => sceneVisibilityMessageID(o.toString()),
  makeCriterion: () => new VisibilityCriterion(),
});

export class VisibilityCriterion extends MultiStringCriterion {
  constructor() {
    super(VisibilityCriterionOption);
  }

  private convertLegacyValues() {
    this.value = this.value.map((v) => legacyVisibilities[v] ?? v);
  }

  protected decodeValue(v: unknown) {
    super.decodeValue(v);
    this.convertLegacyValues();
  }

  public setFromSavedCriterion(criterion: unknown) {
    super.setFromSavedCriterion(criterion);
    this.convertLegacyValues();
  }

  protected getLabelValue(intl: IntlShape) {
    return this.value
      .map((v) => VisibilityCriterionOption.getOptionLabel(intl, v))
      .join(", ");
  }

  public toCriterionInput(): SceneVisibilityCriterionInput {
    return {
      value: this.value as SceneVisibility[],
      modifier: this.modifier,
    };
  }
}
