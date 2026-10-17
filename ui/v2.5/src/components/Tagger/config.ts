import { useCallback, useState } from "react";
import { useConfigurationContext } from "src/hooks/Config";
import { initialConfig, ITaggerConfig } from "./constants";
import { useConfigureUISetting } from "src/core/StashService";
import { usePermissions } from "src/hooks/Permissions";

export function useTaggerConfig() {
  const { configuration: stashConfig } = useConfigurationContext();
  const [saveUISetting] = useConfigureUISetting();
  // only administrators can save the UI configuration. Changes made by other
  // users are kept in local state, and are lost when the page is left.
  const { isAdmin } = usePermissions();
  const [localConfig, setLocalConfig] = useState<ITaggerConfig>();

  const config = localConfig ?? stashConfig?.ui.taggerConfig ?? initialConfig;

  const setConfig = useCallback(
    (c: ITaggerConfig) => {
      if (!isAdmin) {
        setLocalConfig(c);
        return;
      }

      saveUISetting({ variables: { key: "taggerConfig", value: c } });
    },
    [saveUISetting, isAdmin]
  );

  return { config, setConfig };
}
