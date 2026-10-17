import { ApolloError } from "@apollo/client";
import { RoleEnum } from "src/core/generated-graphql";

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
// role required for an operation
const permissionDeniedCode = "PERMISSION_DENIED";

// returns the role required for the operation if the error is a permission
// denied error that names it, or undefined otherwise
export function permissionDeniedRole(error: unknown): RoleEnum | undefined {
  if (!(error instanceof ApolloError)) {
    return undefined;
  }

  const denied = error.graphQLErrors.find(
    (e) => e.extensions?.code === permissionDeniedCode
  );
  const role = denied?.extensions?.requiredRole;
  return typeof role === "string" && role ? (role as RoleEnum) : undefined;
}
