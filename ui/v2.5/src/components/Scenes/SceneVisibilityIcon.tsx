import React from "react";
import { Button, OverlayTrigger, Tooltip } from "react-bootstrap";
import { useIntl } from "react-intl";
import { faLock } from "@fortawesome/free-solid-svg-icons";
import * as GQL from "src/core/generated-graphql";
import { sceneVisibilityMessageID } from "src/utils/sceneVisibility";
import { Icon } from "../Shared/Icon";

// shows a lock on scenes that are hidden from some users
export const SceneVisibilityIcon: React.FC<{
  visibility?: GQL.SceneVisibility;
}> = ({ visibility }) => {
  const intl = useIntl();

  if (!visibility || visibility === GQL.SceneVisibility.Everyone) return null;

  return (
    <OverlayTrigger
      overlay={
        <Tooltip id="scene-visibility-tooltip">
          {intl.formatMessage({
            id: `${sceneVisibilityMessageID(visibility)}_tooltip`,
          })}
        </Tooltip>
      }
      placement="bottom"
    >
      <div className="scene-visibility">
        <Button className="minimal">
          <Icon icon={faLock} />
        </Button>
      </div>
    </OverlayTrigger>
  );
};
