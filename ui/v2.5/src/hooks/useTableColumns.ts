import { useState } from "react";
import { useConfigureUI } from "src/core/StashService";
import { useConfigurationContext } from "src/hooks/Config";
import { useToast } from "./Toast";
import { usePermissions } from "./Permissions";

export const useTableColumns = (
  tableName: string,
  defaultColumns: string[]
) => {
  const Toast = useToast();

  const { configuration } = useConfigurationContext();
  const [saveUI] = useConfigureUI();
  // only administrators can save the UI configuration. Changes made by other
  // users are kept in local state, and are lost when the page is left.
  const { isAdmin } = usePermissions();
  const [localColumns, setLocalColumns] = useState<string[]>();

  const ui = configuration?.ui;

  const selectedColumns =
    localColumns ?? ui?.tableColumns?.[tableName] ?? defaultColumns;

  async function saveColumns(updatedColumns: string[]) {
    if (!isAdmin) {
      setLocalColumns(updatedColumns);
      return;
    }

    try {
      await saveUI({
        variables: {
          input: {
            ...ui,
            tableColumns: {
              ...ui?.tableColumns,
              [tableName]: updatedColumns,
            },
          },
        },
      });
    } catch (e) {
      Toast.error(e);
    }
  }

  return { selectedColumns, saveColumns };
};
