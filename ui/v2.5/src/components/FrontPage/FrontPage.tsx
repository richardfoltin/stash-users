import React, { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { useConfigureUI } from "src/core/StashService";
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
import { usePermissions } from "src/hooks/Permissions";

const FrontPage: React.FC = PatchComponent("FrontPage", () => {
  const intl = useIntl();
  const Toast = useToast();
  const { isAdmin } = usePermissions();

  const [isEditing, setIsEditing] = useState(false);
  const [saving, setSaving] = useState(false);

  const [saveUI] = useConfigureUI();

  const { configuration } = useConfigurationContext();

  useScrollToTopOnMount();

  async function onUpdateConfig(content?: FrontPageContent[]) {
    setIsEditing(false);

    if (!content) {
      return;
    }

    setSaving(true);
    try {
      await saveUI({
        variables: {
          input: {
            ...configuration?.ui,
            frontPageContent: content,
          },
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

  let frontPageContent = getFrontPageContent(ui);

  if (!ui.frontPageContent) {
    const defaultContent = generateDefaultFrontPageContent(intl);
    if (isAdmin) {
      onUpdateConfig(defaultContent);
    } else {
      frontPageContent = defaultContent;
    }
  }

  return (
    <div className="recommendations-container">
      <div>
        {frontPageContent?.map((content, i) => (
          <Control key={i} content={content} />
        ))}
      </div>
      {isAdmin && (
        <div className="recommendations-footer">
          <Button onClick={() => setIsEditing(true)}>
            <FormattedMessage id={"actions.customise"} />
          </Button>
        </div>
      )}
    </div>
  );
});

export default FrontPage;
