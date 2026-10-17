import React from "react";
import { NumberSetting } from "./Inputs";
import { SettingSection } from "./SettingSection";
import { Button } from "react-bootstrap";
import { useIntl } from "react-intl";
import { useSettings } from "./context";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { useToast } from "src/hooks/Toast";
import { useGenerateAPIKey } from "src/core/StashService";
import { usePermissions } from "src/hooks/Permissions";
import { SettingsUsers } from "./SettingsUsers";

export const SettingsSecurityPanel: React.FC = () => {
  const intl = useIntl();
  const Toast = useToast();

  const { general, apiKey, loading, error, saveGeneral, refetch } =
    useSettings();
  const { user } = usePermissions();

  const [generateAPIKey] = useGenerateAPIKey();

  async function onGenerateAPIKey() {
    try {
      await generateAPIKey({
        variables: {
          input: {},
        },
      });
      refetch();
    } catch (e) {
      Toast.error(e);
    }
  }

  async function onClearAPIKey() {
    try {
      await generateAPIKey({
        variables: {
          input: {
            clear: true,
          },
        },
      });
      refetch();
    } catch (e) {
      Toast.error(e);
    }
  }

  if (error) return <h1>{error.message}</h1>;
  if (loading) return <LoadingIndicator />;

  return (
    <>
      <SettingsUsers />
      <SettingSection headingID="config.general.auth.authentication">
        <div className="setting" id="apikey">
          <div>
            <h3>{intl.formatMessage({ id: "config.general.auth.api_key" })}</h3>

            <div className="value text-break">{apiKey}</div>

            <div className="sub-heading">
              {intl.formatMessage({ id: "config.general.auth.api_key_desc" })}
            </div>
          </div>
          <div>
            <Button disabled={!user} onClick={() => onGenerateAPIKey()}>
              {intl.formatMessage({
                id: "config.general.auth.generate_api_key",
              })}
            </Button>
            <Button variant="danger" onClick={() => onClearAPIKey()}>
              {intl.formatMessage({
                id: "config.general.auth.clear_api_key",
              })}
            </Button>
          </div>
        </div>

        <NumberSetting
          id="maxSessionAge"
          headingID="config.general.auth.maximum_session_age"
          subHeadingID="config.general.auth.maximum_session_age_desc"
          value={general.maxSessionAge ?? undefined}
          onChange={(v) => saveGeneral({ maxSessionAge: v })}
        />
      </SettingSection>
    </>
  );
};
