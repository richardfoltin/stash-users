import React, { useMemo } from "react";
import * as GQL from "src/core/generated-graphql";

interface IPermissions {
  // undefined if authentication is not configured
  user?: GQL.UserDataFragment;
  // undefined if the user is locked
  role?: GQL.RoleEnum;
  isAdmin: boolean;
  canModify: boolean;
  canDelete: boolean;
}

// roles from most to least privileged
export const roleOrder = [
  GQL.RoleEnum.Admin,
  GQL.RoleEnum.Modify,
  GQL.RoleEnum.Read,
];

// returns the most privileged role of the user, or undefined if the user has
// no roles
export function getUserRole(
  user: GQL.UserDataFragment
): GQL.RoleEnum | undefined {
  return roleOrder.find((r) => user.roles?.includes(r));
}

// returns the message id of the name of the role
export function roleMessageID(role: GQL.RoleEnum | undefined) {
  return role
    ? `permissions.roles.${role.toLowerCase()}`
    : "permissions.locked";
}

function permissionsForRole(
  role: GQL.RoleEnum | undefined,
  user?: GQL.UserDataFragment
): IPermissions {
  const isAdmin = role === GQL.RoleEnum.Admin;
  const canModify = isAdmin || role === GQL.RoleEnum.Modify;

  return {
    user,
    role,
    isAdmin,
    canModify,
    canDelete: isAdmin,
  };
}

// Without a user, authentication is not configured and everything is allowed.
// If the user could not be loaded, only viewing is allowed.
export function getPermissions(
  user: GQL.UserDataFragment | null | undefined,
  loadFailed?: boolean
): IPermissions {
  if (loadFailed) {
    return permissionsForRole(GQL.RoleEnum.Read);
  }

  if (!user) {
    return permissionsForRole(GQL.RoleEnum.Admin);
  }

  return permissionsForRole(getUserRole(user), user);
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
