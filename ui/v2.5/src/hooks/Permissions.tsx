import React, { useMemo } from "react";
import { IntlShape } from "react-intl";
import * as GQL from "src/core/generated-graphql";

interface IPermissions {
  // undefined if authentication is not configured
  user?: GQL.UserDataFragment;
  loadFailed?: boolean;
  isAdmin: boolean;
  // returns true if the user may do what the permission allows
  can: (p: GQL.Permission) => boolean;
}

// returns the name of the role: the localized name of the Admin role, or
// Locked if the user has no role
export function roleName(
  role: GQL.RoleDataFragment | null | undefined,
  intl: IntlShape
): string {
  if (!role) {
    return intl.formatMessage({ id: "permissions.locked" });
  }

  if (role.builtin) {
    return intl.formatMessage({ id: "permissions.roles.admin" });
  }

  return role.name;
}

// Without a user, authentication is not configured and everything is allowed.
// If the user could not be loaded, only viewing is allowed.
export function getPermissions(
  user: GQL.UserDataFragment | null | undefined,
  loadFailed?: boolean
): IPermissions {
  if (loadFailed) {
    return { loadFailed, isAdmin: false, can: () => false };
  }

  if (!user) {
    return { isAdmin: true, can: () => true };
  }

  const permissions = new Set(user.permissions);
  return {
    user,
    isAdmin: user.isAdmin,
    can: (p) => user.isAdmin || permissions.has(p),
  };
}

const PermissionsContext = React.createContext<IPermissions>(
  getPermissions(undefined)
);

export const PermissionsProvider: React.FC<{
  user: GQL.UserDataFragment | null | undefined;
  loadFailed?: boolean;
}> = ({ user, loadFailed, children }) => {
  const value = useMemo(
    () => getPermissions(user, loadFailed),
    [user, loadFailed]
  );

  return (
    <PermissionsContext.Provider value={value}>
      {children}
    </PermissionsContext.Provider>
  );
};

export const usePermissions = () => React.useContext(PermissionsContext);
