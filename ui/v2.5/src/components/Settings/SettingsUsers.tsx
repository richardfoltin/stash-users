import React, { useState } from "react";
import { Badge, Button, Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import {
  faExclamationTriangle,
  faInfoCircle,
  faTrashAlt,
} from "@fortawesome/free-solid-svg-icons";
import * as GQL from "src/core/generated-graphql";
import {
  useChangeUserPassword,
  useRoles,
  useUserCreate,
  useUserDestroy,
  useUserUpdate,
  useUsers,
} from "src/core/StashService";
import { roleName, usePermissions } from "src/hooks/Permissions";
import { useToast } from "src/hooks/Toast";
import { maxPasswordBytes, passwordTooLong } from "src/utils/password";
import { ModalComponent } from "../Shared/Modal";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { Icon } from "../Shared/Icon";
import { SettingModal } from "./Inputs";
import { SettingSection } from "./SettingSection";

function roleBadgeVariant(role: GQL.RoleDataFragment | null | undefined) {
  if (!role) return "danger";
  return role.builtin ? "primary" : "secondary";
}

// returns the role of new users: the role with the fewest permissions
function defaultRoleID(roles: GQL.RoleDataFragment[]) {
  const least = roles
    .filter((r) => !r.builtin)
    .reduce<GQL.RoleDataFragment | undefined>(
      (ret, r) =>
        !ret || r.permissions.length < ret.permissions.length ? r : ret,
      undefined
    );

  return (least ?? roles[0])?.id ?? "";
}

function validPassword(password: string | undefined) {
  return !!password && !passwordTooLong(password);
}

const RoleSelect: React.FC<{
  roles: GQL.RoleDataFragment[];
  value: string;
  onChange: (v: string) => void;
}> = ({ roles, value, onChange }) => {
  const intl = useIntl();
  const selected = roles.find((r) => r.id === value);

  return (
    <Form.Group id="user-role">
      <h6>{intl.formatMessage({ id: "config.general.auth.users.role" })}</h6>
      <Form.Control
        as="select"
        className="input-control"
        value={value}
        onChange={(e: React.ChangeEvent<HTMLSelectElement>) =>
          onChange(e.currentTarget.value)
        }
      >
        {roles.map((r) => (
          <option key={r.id} value={r.id}>
            {roleName(r, intl)}
          </option>
        ))}
      </Form.Control>
      {selected && (
        <Form.Text className="text-muted">{selected.description}</Form.Text>
      )}
    </Form.Group>
  );
};

const PasswordInput: React.FC<{
  value: string;
  onChange: (v: string) => void;
}> = ({ value, onChange }) => {
  const intl = useIntl();
  const tooLong = passwordTooLong(value);

  return (
    <Form.Group id="user-password">
      <h6>{intl.formatMessage({ id: "config.general.auth.password" })}</h6>
      <Form.Control
        className="text-input"
        type="password"
        autoComplete="new-password"
        value={value}
        isInvalid={tooLong}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
          onChange(e.currentTarget.value)
        }
      />
      <Form.Control.Feedback type="invalid">
        {tooLong &&
          intl.formatMessage(
            { id: "config.general.auth.users.password_too_long" },
            { max: maxPasswordBytes }
          )}
      </Form.Control.Feedback>
    </Form.Group>
  );
};

const AutoLoginInput: React.FC<{
  value: boolean;
  isAdmin: boolean;
  disabledReason?: string;
  onChange: (v: boolean) => void;
}> = ({ value, isAdmin, disabledReason, onChange }) => {
  const intl = useIntl();

  return (
    <Form.Group id="user-auto-login">
      <Form.Switch
        id="user-auto-login-check"
        checked={value}
        disabled={!!disabledReason}
        label={intl.formatMessage({
          id: "config.general.auth.users.auto_login",
        })}
        onChange={() => onChange(!value)}
      />
      <Form.Text className="text-muted">
        {disabledReason ??
          intl.formatMessage({
            id: "config.general.auth.users.auto_login_desc",
          })}
      </Form.Text>
      {value && isAdmin && (
        <Form.Text className="text-warning">
          {intl.formatMessage({
            id: "config.general.auth.users.auto_login_admin_warning",
          })}
        </Form.Text>
      )}
    </Form.Group>
  );
};

interface IUserInput {
  name: string;
  password: string;
  roleId: string;
  autoLogin: boolean;
}

const AddUserModal: React.FC<{
  roles: GQL.RoleDataFragment[];
  autoLoginDisabledReason?: string;
  close: (v?: IUserInput) => void;
}> = ({ roles, autoLoginDisabledReason, close }) => {
  const intl = useIntl();

  return (
    <SettingModal<IUserInput>
      headingID="config.general.auth.users.add_user"
      value={{
        name: "",
        password: "",
        roleId: defaultRoleID(roles),
        autoLogin: false,
      }}
      validate={(v) => v.name.trim() !== "" && validPassword(v.password)}
      renderField={(v, setValue) => (
        <>
          <Form.Group id="user-name">
            <h6>
              {intl.formatMessage({ id: "config.general.auth.username" })}
            </h6>
            <Form.Control
              className="text-input"
              autoComplete="off"
              value={v?.name ?? ""}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                setValue({ ...v!, name: e.currentTarget.value })
              }
            />
          </Form.Group>
          <PasswordInput
            value={v?.password ?? ""}
            onChange={(password) => setValue({ ...v!, password })}
          />
          <RoleSelect
            roles={roles}
            value={v?.roleId ?? ""}
            onChange={(roleId) => setValue({ ...v!, roleId })}
          />
          <AutoLoginInput
            value={v?.autoLogin ?? false}
            isAdmin={!!roles.find((r) => r.id === v?.roleId)?.builtin}
            disabledReason={autoLoginDisabledReason}
            onChange={(autoLogin) => setValue({ ...v!, autoLogin })}
          />
        </>
      )}
      close={close}
    />
  );
};

export const SettingsUsers: React.FC = () => {
  const intl = useIntl();
  const Toast = useToast();
  const { user: currentUser } = usePermissions();

  const { data, loading, error } = useUsers();
  const { data: rolesData } = useRoles();
  const [createUser] = useUserCreate();
  const [updateUser] = useUserUpdate();
  const [destroyUser] = useUserDestroy();
  const [changeUserPassword] = useChangeUserPassword();

  const [isCreating, setIsCreating] = useState(false);
  const [editingRole, setEditingRole] = useState<GQL.UserDataFragment>();
  const [editingPassword, setEditingPassword] =
    useState<GQL.UserDataFragment>();
  const [deleting, setDeleting] = useState<GQL.UserDataFragment>();

  const users = data?.users ?? [];
  const roles = rolesData?.roles ?? [];

  // a user without a password can only log in automatically, so auto login
  // cannot move away from that user
  const autoLoginUser = users.find((u) => u.autoLogin);
  function autoLoginDisabledReason(u?: GQL.UserDataFragment) {
    if (!autoLoginUser || autoLoginUser.hasPassword) return;

    if (u?.name === autoLoginUser.name) {
      return intl.formatMessage({
        id: "config.general.auth.users.auto_login_no_password",
      });
    }

    return intl.formatMessage(
      { id: "config.general.auth.users.auto_login_other_no_password" },
      { name: autoLoginUser.name }
    );
  }

  async function onCreate(v: IUserInput) {
    try {
      await createUser({
        variables: {
          input: {
            name: v.name.trim(),
            password: v.password,
            roleId: v.roleId,
            autoLogin: v.autoLogin,
          },
        },
      });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.users.user_added" },
          { name: v.name.trim() }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onChangeRole(u: GQL.UserDataFragment, roleId: string) {
    try {
      await updateUser({
        variables: {
          input: {
            existingName: u.name,
            name: u.name,
            roleId,
          },
        },
      });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.users.user_updated" },
          { name: u.name }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onChangeAutoLogin(
    u: GQL.UserDataFragment,
    autoLogin: boolean
  ) {
    try {
      await updateUser({
        variables: {
          input: {
            existingName: u.name,
            name: u.name,
            roleId: u.role?.id,
            autoLogin,
          },
        },
      });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.users.user_updated" },
          { name: u.name }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onChangePassword(u: GQL.UserDataFragment, password: string) {
    try {
      await changeUserPassword({
        variables: {
          input: {
            name: u.name,
            newPassword: password,
          },
        },
      });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.users.password_changed" },
          { name: u.name }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onDelete(u: GQL.UserDataFragment) {
    try {
      await destroyUser({
        variables: {
          input: {
            name: u.name,
          },
        },
      });
      Toast.success(
        intl.formatMessage(
          { id: "config.general.auth.users.user_removed" },
          { name: u.name }
        )
      );
    } catch (e) {
      Toast.error(e);
    }
  }

  function renderUsers() {
    if (error) {
      return (
        <div className="setting">
          <div className="text-danger">{error.message}</div>
        </div>
      );
    }

    if (loading) return <LoadingIndicator inline small />;

    const adminCount = users.filter((u) => u.isAdmin).length;

    return users.map((u, i) => {
      const isLastAdmin = u.isAdmin && adminCount <= 1;
      const isCurrentUser = u.name === currentUser?.name;
      const autoLoginDisabled = autoLoginDisabledReason(u);

      return (
        <div key={u.name} className="setting user-setting">
          <div>
            <h3>
              {u.name}
              {isCurrentUser && (
                <span className="user-current">
                  <FormattedMessage id="config.general.auth.users.current_user" />
                </span>
              )}
            </h3>
            <div className="user-badges">
              <Badge className="user-badge" variant={roleBadgeVariant(u.role)}>
                {roleName(u.role, intl)}
              </Badge>
              {u.autoLogin && (
                <Badge className="user-badge" variant="info">
                  <FormattedMessage id="config.general.auth.users.auto_login_enabled" />
                </Badge>
              )}
              {!u.hasPassword && (
                <Badge className="user-badge" variant="warning">
                  <FormattedMessage id="config.general.auth.users.no_password" />
                </Badge>
              )}
            </div>
            <Form.Switch
              id={`user-auto-login-${i}`}
              className="user-auto-login-switch"
              checked={u.autoLogin}
              disabled={!!autoLoginDisabled}
              label={intl.formatMessage({
                id: "config.general.auth.users.auto_login",
              })}
              onChange={() => onChangeAutoLogin(u, !u.autoLogin)}
            />
            {autoLoginDisabled && (
              <div className="user-note">
                <Icon icon={faInfoCircle} />
                {autoLoginDisabled}
              </div>
            )}
            {u.autoLogin && u.isAdmin && (
              <div className="user-note text-warning">
                <Icon icon={faExclamationTriangle} />
                <FormattedMessage id="config.general.auth.users.auto_login_admin_warning" />
              </div>
            )}
            {isLastAdmin && (
              <div className="user-note">
                <Icon icon={faInfoCircle} />
                <FormattedMessage id="config.general.auth.users.last_admin" />
              </div>
            )}
          </div>
          <div>
            <Button
              disabled={isLastAdmin || !roles.length}
              title={
                isLastAdmin
                  ? intl.formatMessage({
                      id: "config.general.auth.users.last_admin",
                    })
                  : undefined
              }
              onClick={() => setEditingRole(u)}
            >
              <FormattedMessage id="config.general.auth.users.change_role" />
            </Button>
            <Button onClick={() => setEditingPassword(u)}>
              <FormattedMessage id="permissions.change_password" />
            </Button>
            <Button
              variant="danger"
              disabled={isLastAdmin}
              title={
                isLastAdmin
                  ? intl.formatMessage({
                      id: "config.general.auth.users.last_admin",
                    })
                  : undefined
              }
              onClick={() => setDeleting(u)}
            >
              <FormattedMessage id="actions.remove" />
            </Button>
          </div>
        </div>
      );
    });
  }

  return (
    <SettingSection
      id="users"
      headingID="config.general.auth.users.heading"
      subHeadingID="config.general.auth.users.description"
    >
      {isCreating && (
        <AddUserModal
          roles={roles}
          autoLoginDisabledReason={autoLoginDisabledReason()}
          close={(v) => {
            setIsCreating(false);
            if (v) onCreate(v);
          }}
        />
      )}

      {editingRole && (
        <SettingModal<string>
          heading={intl.formatMessage(
            { id: "config.general.auth.users.change_role_for" },
            { name: editingRole.name }
          )}
          value={editingRole.role?.id ?? defaultRoleID(roles)}
          renderField={(v, setValue) => (
            <RoleSelect roles={roles} value={v ?? ""} onChange={setValue} />
          )}
          close={(v) => {
            const u = editingRole;
            setEditingRole(undefined);
            if (v && v !== u.role?.id) onChangeRole(u, v);
          }}
        />
      )}

      {editingPassword && (
        <SettingModal<string>
          heading={intl.formatMessage(
            { id: "config.general.auth.users.change_password_for" },
            { name: editingPassword.name }
          )}
          value=""
          validate={(v) => validPassword(v)}
          renderField={(v, setValue) => (
            <PasswordInput value={v ?? ""} onChange={setValue} />
          )}
          close={(v) => {
            const u = editingPassword;
            setEditingPassword(undefined);
            if (v) onChangePassword(u, v);
          }}
        />
      )}

      <ModalComponent
        show={!!deleting}
        icon={faTrashAlt}
        header={intl.formatMessage({
          id: "config.general.auth.users.remove_user",
        })}
        accept={{
          text: intl.formatMessage({ id: "actions.remove" }),
          variant: "danger",
          onClick: () => {
            const u = deleting;
            setDeleting(undefined);
            if (u) onDelete(u);
          },
        }}
        cancel={{
          onClick: () => setDeleting(undefined),
          variant: "secondary",
        }}
      >
        <p>
          <FormattedMessage
            id="config.general.auth.users.remove_user_confirm"
            values={{ name: deleting?.name }}
          />
        </p>
      </ModalComponent>

      {renderUsers()}

      <div className="setting">
        <div>
          <div className="sub-heading">
            <FormattedMessage id="config.general.auth.users.auto_login_section_desc" />
          </div>
        </div>
        <div>
          <Button disabled={!roles.length} onClick={() => setIsCreating(true)}>
            <FormattedMessage id="config.general.auth.users.add_user" />
          </Button>
        </div>
      </div>
    </SettingSection>
  );
};
