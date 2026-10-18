import React from "react";
import { Form, Table } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import * as GQL from "src/core/generated-graphql";
import {
  objectPermissionRows,
  otherPermissions,
  permissionDescriptionMessageID,
  permissionMessageID,
} from "src/utils/permissions";
import { SettingModal } from "./Inputs";

export interface IRoleInput {
  name: string;
  description: string;
  permissions: GQL.Permission[];
}

function togglePermission(v: IRoleInput, p: GQL.Permission): IRoleInput {
  const permissions = v.permissions.includes(p)
    ? v.permissions.filter((q) => q !== p)
    : [...v.permissions, p];

  return { ...v, permissions };
}

const PermissionCheck: React.FC<{
  permission?: GQL.Permission;
  value: IRoleInput;
  onChange: (v: IRoleInput) => void;
  label?: React.ReactNode;
}> = ({ permission, value, onChange, label }) => {
  const intl = useIntl();

  if (!permission) return null;

  return (
    <Form.Check
      id={`role-permission-${permission.toLowerCase()}`}
      checked={value.permissions.includes(permission)}
      label={label}
      aria-label={intl.formatMessage({ id: permissionMessageID(permission) })}
      onChange={() => onChange(togglePermission(value, permission))}
    />
  );
};

// edits a new role, if role is not set
export const RoleEditModal: React.FC<{
  role?: GQL.RoleDataFragment;
  // the existing roles, whose names cannot be used again
  roles: GQL.RoleDataFragment[];
  heading: string;
  close: (v?: IRoleInput) => void;
}> = ({ role, roles, heading, close }) => {
  const intl = useIntl();

  // role names are unique regardless of case
  function nameExists(name: string) {
    const n = name.trim().toLowerCase();
    return roles.some(
      (r) => r.id !== role?.id && r.name.trim().toLowerCase() === n
    );
  }

  function renderField(v: IRoleInput, setValue: (v: IRoleInput) => void) {
    return (
      <>
        <Form.Group id="role-name">
          <h6>
            <FormattedMessage id="name" />
          </h6>
          <Form.Control
            className="text-input"
            autoComplete="off"
            value={v.name}
            isInvalid={nameExists(v.name)}
            onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
              setValue({ ...v, name: e.currentTarget.value })
            }
          />
          <Form.Control.Feedback type="invalid">
            {nameExists(v.name) && (
              <FormattedMessage id="config.general.auth.roles.name_exists" />
            )}
          </Form.Control.Feedback>
        </Form.Group>
        <Form.Group id="role-description">
          <h6>
            <FormattedMessage id="description" />
          </h6>
          <Form.Control
            as="textarea"
            rows={3}
            className="text-input"
            value={v.description}
            onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
              setValue({ ...v, description: e.currentTarget.value })
            }
          />
        </Form.Group>

        <Table size="sm" className="role-object-permissions">
          <thead>
            <tr>
              <th>
                <FormattedMessage id="config.general.auth.roles.content" />
              </th>
              <th>
                <FormattedMessage id="actions.edit" />
              </th>
              <th>
                <FormattedMessage id="actions.delete" />
              </th>
              <th>
                <FormattedMessage id="actions.merge" />
              </th>
            </tr>
          </thead>
          <tbody>
            {objectPermissionRows.map((row) => (
              <tr key={row.object}>
                <td>
                  <FormattedMessage id={row.object} />
                </td>
                <td>
                  <PermissionCheck
                    permission={row.edit}
                    value={v}
                    onChange={setValue}
                  />
                </td>
                <td>
                  <PermissionCheck
                    permission={row.delete}
                    value={v}
                    onChange={setValue}
                  />
                </td>
                <td>
                  <PermissionCheck
                    permission={row.merge}
                    value={v}
                    onChange={setValue}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </Table>

        <div className="role-other-permissions">
          <h6>
            <FormattedMessage id="config.general.auth.roles.other_permissions" />
          </h6>
          {otherPermissions.map((p) => (
            <PermissionCheck
              key={p}
              permission={p}
              value={v}
              onChange={setValue}
              label={
                <>
                  {intl.formatMessage({ id: permissionMessageID(p) })}
                  <span className="role-permission-description">
                    <FormattedMessage id={permissionDescriptionMessageID(p)} />
                  </span>
                </>
              }
            />
          ))}
        </div>
      </>
    );
  }

  return (
    <SettingModal<IRoleInput>
      heading={heading}
      value={{
        name: role?.name ?? "",
        description: role?.description ?? "",
        permissions: role?.permissions ?? [],
      }}
      validate={(v) => v.name.trim() !== "" && !nameExists(v.name)}
      renderField={(v, setValue) => renderField(v!, setValue)}
      close={close}
    />
  );
};
