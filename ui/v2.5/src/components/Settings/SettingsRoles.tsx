import React, { useState } from "react";
import { Button } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { faTrashAlt } from "@fortawesome/free-solid-svg-icons";
import * as GQL from "src/core/generated-graphql";
import {
  useRoleCreate,
  useRoleDestroy,
  useRoleUpdate,
  useRoles,
} from "src/core/StashService";
import { roleName } from "src/hooks/Permissions";
import { useToast } from "src/hooks/Toast";
import { ModalComponent } from "../Shared/Modal";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { SettingSection } from "./SettingSection";
import { IRoleInput, RoleEditModal } from "./RoleEditModal";

export const SettingsRoles: React.FC = () => {
  const intl = useIntl();
  const Toast = useToast();

  const { data, loading, error } = useRoles();
  const [createRole] = useRoleCreate();
  const [updateRole] = useRoleUpdate();
  const [destroyRole] = useRoleDestroy();

  const [isCreating, setIsCreating] = useState(false);
  const [editing, setEditing] = useState<GQL.RoleDataFragment>();
  const [deleting, setDeleting] = useState<GQL.RoleDataFragment>();

  const roles = data?.roles ?? [];

  async function onCreate(v: IRoleInput) {
    try {
      await createRole({ variables: { input: v } });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.roles.role_added" },
          { name: v.name.trim() }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onUpdate(id: string, v: IRoleInput) {
    try {
      await updateRole({ variables: { input: { id, ...v } } });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.roles.role_updated" },
          { name: v.name.trim() }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onDelete(r: GQL.RoleDataFragment) {
    try {
      await destroyRole({ variables: { input: { id: r.id } } });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.roles.role_removed" },
          { name: r.name }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  function renderRoles() {
    if (error) {
      return (
        <div className="setting">
          <div className="text-danger">{error.message}</div>
        </div>
      );
    }

    if (loading) return <LoadingIndicator inline small />;

    return roles.map((r) => {
      const inUse = r.userCount > 0;

      return (
        <div key={r.id} className="setting role-setting">
          <div>
            <h3>{roleName(r, intl)}</h3>
            <div className="sub-heading">{r.description}</div>
            <div className="role-user-count">
              <FormattedMessage
                id="config.general.auth.roles.user_count"
                values={{ count: r.userCount }}
              />
            </div>
          </div>
          <div>
            {r.builtin ? (
              <span className="role-all-permissions">
                <FormattedMessage id="config.general.auth.roles.all_permissions" />
              </span>
            ) : (
              <>
                <Button onClick={() => setEditing(r)}>
                  <FormattedMessage id="actions.edit" />
                </Button>
                <Button
                  variant="danger"
                  disabled={inUse}
                  title={
                    inUse
                      ? intl.formatMessage({
                          id: "config.general.auth.roles.assigned_to_users",
                        })
                      : undefined
                  }
                  onClick={() => setDeleting(r)}
                >
                  <FormattedMessage id="actions.remove" />
                </Button>
              </>
            )}
          </div>
        </div>
      );
    });
  }

  return (
    <SettingSection
      id="roles"
      headingID="config.general.auth.roles.heading"
      subHeadingID="config.general.auth.roles.description"
    >
      {isCreating && (
        <RoleEditModal
          roles={roles}
          heading={intl.formatMessage({
            id: "config.general.auth.roles.new_role",
          })}
          close={(v) => {
            setIsCreating(false);
            if (v) onCreate(v);
          }}
        />
      )}

      {editing && (
        <RoleEditModal
          role={editing}
          roles={roles}
          heading={intl.formatMessage(
            { id: "config.general.auth.roles.edit_role" },
            { name: editing.name }
          )}
          close={(v) => {
            const r = editing;
            setEditing(undefined);
            if (v) onUpdate(r.id, v);
          }}
        />
      )}

      <ModalComponent
        show={!!deleting}
        icon={faTrashAlt}
        header={intl.formatMessage({
          id: "config.general.auth.roles.remove_role",
        })}
        accept={{
          text: intl.formatMessage({ id: "actions.remove" }),
          variant: "danger",
          onClick: () => {
            const r = deleting;
            setDeleting(undefined);
            if (r) onDelete(r);
          },
        }}
        cancel={{
          onClick: () => setDeleting(undefined),
          variant: "secondary",
        }}
      >
        <p>
          <FormattedMessage
            id="config.general.auth.roles.remove_role_confirm"
            values={{ name: deleting?.name }}
          />
        </p>
      </ModalComponent>

      {renderRoles()}

      <div className="setting">
        <div />
        <div>
          <Button onClick={() => setIsCreating(true)}>
            <FormattedMessage id="config.general.auth.roles.new_role" />
          </Button>
        </div>
      </div>
    </SettingSection>
  );
};
