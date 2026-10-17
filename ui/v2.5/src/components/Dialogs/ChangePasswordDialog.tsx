import React, { useState } from "react";
import { Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { faKey } from "@fortawesome/free-solid-svg-icons";
import { getPlatformURL } from "src/core/createClient";
import { useChangePassword } from "src/core/StashService";
import { useToast } from "src/hooks/Toast";
import { maxPasswordBytes, passwordTooLong } from "src/utils/password";
import { ModalComponent } from "../Shared/Modal";

// changing the password ends the user's sessions, so the login page follows
export const ChangePasswordDialog: React.FC<{
  username: string;
  onClose: () => void;
}> = ({ username, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const [changePassword] = useChangePassword();

  const [existingPassword, setExistingPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isRunning, setIsRunning] = useState(false);

  const mismatch = confirmPassword !== "" && newPassword !== confirmPassword;
  const tooLong = passwordTooLong(newPassword);
  const valid =
    existingPassword !== "" &&
    newPassword !== "" &&
    !tooLong &&
    !mismatch &&
    !!confirmPassword;

  async function onSubmit() {
    setIsRunning(true);
    try {
      await changePassword({
        variables: {
          input: {
            existingPassword,
            newPassword,
          },
        },
      });
      Toast.success(
        intl.formatMessage({ id: "dialogs.change_password.success" })
      );
      onClose();
      // leave the toast on screen briefly before the page is left
      setTimeout(() => {
        window.location.href = getPlatformURL("login").toString();
      }, 2000);
    } catch (e) {
      Toast.error(e);
      setIsRunning(false);
    }
  }

  function renderPasswordInput(
    id: string,
    value: string,
    setValue: (v: string) => void,
    autoComplete: string,
    error?: string
  ) {
    return (
      <Form.Group controlId={id}>
        <Form.Label>
          <FormattedMessage id={`dialogs.change_password.${id}`} />
        </Form.Label>
        <Form.Control
          className="text-input"
          type="password"
          autoComplete={autoComplete}
          value={value}
          isInvalid={!!error}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            setValue(e.currentTarget.value)
          }
        />
        <Form.Control.Feedback type="invalid">{error}</Form.Control.Feedback>
      </Form.Group>
    );
  }

  return (
    <ModalComponent
      show
      icon={faKey}
      header={intl.formatMessage(
        { id: "dialogs.change_password.heading" },
        { name: username }
      )}
      accept={{
        text: intl.formatMessage({ id: "actions.confirm" }),
        onClick: () => onSubmit(),
      }}
      cancel={{
        onClick: () => onClose(),
        variant: "secondary",
      }}
      disabled={!valid}
      isRunning={isRunning}
    >
      <Form
        onSubmit={(e) => {
          e.preventDefault();
          if (valid) onSubmit();
        }}
      >
        {renderPasswordInput(
          "current_password",
          existingPassword,
          setExistingPassword,
          "current-password"
        )}
        {renderPasswordInput(
          "new_password",
          newPassword,
          setNewPassword,
          "new-password",
          tooLong
            ? intl.formatMessage(
                { id: "config.general.auth.users.password_too_long" },
                { max: maxPasswordBytes }
              )
            : undefined
        )}
        {renderPasswordInput(
          "confirm_password",
          confirmPassword,
          setConfirmPassword,
          "new-password",
          mismatch
            ? intl.formatMessage({ id: "dialogs.change_password.mismatch" })
            : undefined
        )}
      </Form>
    </ModalComponent>
  );
};
