import { IntlShape } from "react-intl";
import {
  CriterionModifier,
  RoleCriterionInput,
  RoleEnum,
} from "src/core/generated-graphql";
import {
  sceneVisibilities,
  sceneVisibilityMessageID,
} from "src/utils/sceneVisibility";
import { ModifierCriterionOption, MultiStringCriterion } from "./criterion";

export const VisibilityCriterionOption = new ModifierCriterionOption({
  messageID: "scene_visibility.visible_to",
  type: "visibility",
  adminOnly: true,
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

  protected getLabelValue(intl: IntlShape) {
    return this.value
      .map((v) => VisibilityCriterionOption.getOptionLabel(intl, v))
      .join(", ");
  }

  public toCriterionInput(): RoleCriterionInput {
    return {
      value: this.value as RoleEnum[],
      modifier: this.modifier,
    };
  }
}
