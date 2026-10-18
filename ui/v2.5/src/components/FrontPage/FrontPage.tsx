import React, { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { useConfigureUISetting } from "src/core/StashService";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { Button } from "react-bootstrap";
import { FrontPageConfig } from "./FrontPageConfig";
import { useToast } from "src/hooks/Toast";
import { Control } from "./Control";
import { useConfigurationContext } from "src/hooks/Config";
import {
  FrontPageContent,
  generateDefaultFrontPageContent,
  getFrontPageContent,
} from "src/core/config";
import { useScrollToTopOnMount } from "src/hooks/scrollToTop";
import { PatchComponent } from "src/patch";

const FrontPage: React.FC = PatchComponent("FrontPage", () => {
  const intl = useIntl();
  const Toast = useToast();

  const [isEditing, setIsEditing] = useState(false);
  const [saving, setSaving] = useState(false);

  const [saveUISetting] = useConfigureUISetting();

  const { configuration } = useConfigurationContext();

  useScrollToTopOnMount();

  async function onUpdateConfig(content?: FrontPageContent[]) {
    setIsEditing(false);

    if (!content) {
      return;
    }

    setSaving(true);
    try {
      await saveUISetting({
        variables: {
          key: "frontPageContent",
          value: content,
        },
      });
    } catch (e) {
      Toast.error(e);
    }
    setSaving(false);
  }

  if (saving) {
    return <LoadingIndicator />;
  }

  if (isEditing) {
    return <FrontPageConfig onClose={(content) => onUpdateConfig(content)} />;
  }

  const ui = configuration?.ui ?? {};

  // the generated front page is shown, but not saved, so that a published
  // front page reaches every user who has not customised their own
  const frontPageContent =
    getFrontPageContent(ui) ?? generateDefaultFrontPageContent(intl);

  return (
    <div className="recommendations-container">
      <div>
        {frontPageContent?.map((content, i) => (
          <Control key={i} content={content} />
        ))}
      </div>
      <div className="recommendations-footer">
        <Button onClick={() => setIsEditing(true)}>
          <FormattedMessage id={"actions.customise"} />
        </Button>
      </div>
    </div>
  );
});

export default FrontPage;
