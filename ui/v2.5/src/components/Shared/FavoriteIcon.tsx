import React from "react";
import { Icon } from "../Shared/Icon";
import { Button } from "react-bootstrap";
import { faHeart } from "@fortawesome/free-solid-svg-icons";
import cx from "classnames";
import { SizeProp } from "@fortawesome/fontawesome-svg-core";
import { usePermissions } from "src/hooks/Permissions";

export const FavoriteIcon: React.FC<{
  favorite: boolean;
  onToggleFavorite: (v: boolean) => void;
  size?: SizeProp;
  className?: string;
}> = ({ favorite, onToggleFavorite, size, className }) => {
  const { canModify } = usePermissions();

  return (
    <Button
      className={cx(
        "minimal",
        "mousetrap",
        "favorite-button",
        className,
        favorite ? "favorite" : "not-favorite"
      )}
      onClick={() => onToggleFavorite!(!favorite)}
      disabled={!canModify}
    >
      <Icon icon={faHeart} size={size} />
    </Button>
  );
};
