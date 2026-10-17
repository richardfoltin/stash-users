import { ApolloError } from "@apollo/client";
import { Permission } from "src/core/generated-graphql";

export const apolloError = (error: unknown) =>
  error instanceof ApolloError ? error.message : "";

export function errorToString(error: unknown) {
  let message = "";
  if (error instanceof Error) {
    message = error.message;
  }
  if (!message) {
    message = String(error);
  }
  if (!message) {
    message = "Unknown error";
  }

  return message;
}

// error code returned by the server when the current user does not have the
// permission or the role required for an operation
const permissionDeniedCode = "PERMISSION_DENIED";

interface IPermissionDenied {
  // the permission that the operation requires, or undefined if it requires
  // the Admin role
  permission?: Permission;
}

// returns what the operation requires if the error is a permission denied
// error, or undefined otherwise
export function permissionDenied(
  error: unknown
): IPermissionDenied | undefined {
  if (!(error instanceof ApolloError)) {
    return undefined;
  }

  const denied = error.graphQLErrors.find(
    (e) => e.extensions?.code === permissionDeniedCode
  );
  if (!denied) {
    return undefined;
  }

  const permission = denied.extensions?.requiredPermission;
  return typeof permission === "string" && permission
    ? { permission: permission as Permission }
    : {};
}
