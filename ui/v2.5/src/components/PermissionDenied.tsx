import React from "react";
import { FormattedMessage } from "react-intl";

export const PermissionDenied: React.FC = () => {
  return (
    <h1>
      <FormattedMessage id="permissions.page_denied" />
    </h1>
  );
};
