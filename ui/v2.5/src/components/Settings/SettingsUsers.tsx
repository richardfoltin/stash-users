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
  useUserCreate,
  useUserDestroy,
  useUserUpdate,
  useUsers,
} from "src/core/StashService";
import {
  getUserRole,
  roleMessageID,
  roleOrder,
  usePermissions,
} from "src/hooks/Permissions";
import { useToast } from "src/hooks/Toast";
import { maxPasswordBytes, passwordTooLong } from "src/utils/password";
import { ModalComponent } from "../Shared/Modal";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { Icon } from "../Shared/Icon";
import { SettingModal } from "./Inputs";
import { SettingSection } from "./SettingSection";

const roleBadgeVariants = {
  [GQL.RoleEnum.Admin]: "primary",
  [GQL.RoleEnum.Modify]: "success",
  [GQL.RoleEnum.Read]: "secondary",
};

function validPassword(password: string | undefined) {
  return !!password && !passwordTooLong(password);
}

const RoleSelect: React.FC<{
  value: GQL.RoleEnum;
  onChange: (v: GQL.RoleEnum) => void;
}> = ({ value, onChange }) => {
  const intl = useIntl();

  return (
    <Form.Group id="user-role">
      <h6>{intl.formatMessage({ id: "config.general.auth.users.role" })}</h6>
      <Form.Control
        as="select"
        className="input-control"
        value={value}
        onChange={(e: React.ChangeEvent<HTMLSelectElement>) =>
          onChange(e.currentTarget.value as GQL.RoleEnum)
        }
      >
        {roleOrder.map((r) => (
          <option key={r} value={r}>
            {intl.formatMessage({ id: roleMessageID(r) })}
          </option>
        ))}
      </Form.Control>
      <Form.Text className="text-muted">
        {intl.formatMessage({
          id: `config.general.auth.users.role_desc.${value.toLowerCase()}`,
        })}
      </Form.Text>
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
  role: GQL.RoleEnum | undefined;
  disabledReason?: string;
  onChange: (v: boolean) => void;
}> = ({ value, role, disabledReason, onChange }) => {
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
      {value && role === GQL.RoleEnum.Admin && (
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
  role: GQL.RoleEnum;
  autoLogin: boolean;
}

const AddUserModal: React.FC<{
  autoLoginDisabledReason?: string;
  close: (v?: IUserInput) => void;
}> = ({ autoLoginDisabledReason, close }) => {
  const intl = useIntl();

  return (
    <SettingModal<IUserInput>
      headingID="config.general.auth.users.add_user"
      value={{
        name: "",
        password: "",
        role: GQL.RoleEnum.Read,
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
            value={v?.role ?? GQL.RoleEnum.Read}
            onChange={(role) => setValue({ ...v!, role })}
          />
          <AutoLoginInput
            value={v?.autoLogin ?? false}
            role={v?.role}
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
            roles: [v.role],
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

  async function onChangeRole(u: GQL.UserDataFragment, role: GQL.RoleEnum) {
    try {
      await updateUser({
        variables: {
          input: {
            existingName: u.name,
            name: u.name,
            roles: [role],
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
            roles: u.roles ?? [],
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

    const adminCount = users.filter(
      (u) => getUserRole(u) === GQL.RoleEnum.Admin
    ).length;

    return users.map((u, i) => {
      const role = getUserRole(u);
      const isLastAdmin = role === GQL.RoleEnum.Admin && adminCount <= 1;
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
              <Badge
                className="user-badge"
                variant={role ? roleBadgeVariants[role] : "danger"}
              >
                <FormattedMessage id={roleMessageID(role)} />
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
            {u.autoLogin && role === GQL.RoleEnum.Admin && (
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
              disabled={isLastAdmin}
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
          autoLoginDisabledReason={autoLoginDisabledReason()}
          close={(v) => {
            setIsCreating(false);
            if (v) onCreate(v);
          }}
        />
      )}

      {editingRole && (
        <SettingModal<GQL.RoleEnum>
          heading={intl.formatMessage(
            { id: "config.general.auth.users.change_role_for" },
            { name: editingRole.name }
          )}
          value={getUserRole(editingRole) ?? GQL.RoleEnum.Read}
          renderField={(v, setValue) => (
            <RoleSelect value={v ?? GQL.RoleEnum.Read} onChange={setValue} />
          )}
          close={(v) => {
            const u = editingRole;
            setEditingRole(undefined);
            if (v && v !== getUserRole(u)) onChangeRole(u, v);
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
          <Button onClick={() => setIsCreating(true)}>
            <FormattedMessage id="config.general.auth.users.add_user" />
          </Button>
        </div>
      </div>
    </SettingSection>
  );
};
