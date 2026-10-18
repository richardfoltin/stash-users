import React, { useCallback, useContext, useMemo, useState } from "react";
import { Badge, Button, Form } from "react-bootstrap";
import { FormattedMessage, IntlShape, useIntl } from "react-intl";
import { faRotateLeft } from "@fortawesome/free-solid-svg-icons";
import * as GQL from "src/core/generated-graphql";
import {
  useFindSavedFilters,
  usePersonalSettings,
  usePublishPersonalSettings,
  useResetPersonalSettings,
} from "src/core/StashService";
import { FrontPageContent } from "src/core/config";
import { usePermissions } from "src/hooks/Permissions";
import { useToast } from "src/hooks/Toast";
import { AlertModal } from "../Shared/Alert";
import { Icon } from "../Shared/Icon";
import { ModalComponent } from "../Shared/Modal";
import { SettingSection } from "./SettingSection";
import { useSettingsOptional } from "./context";

// identifies a personal setting: a setting that is inherited, reset and
// published as a whole
export interface IPersonalSettingKey {
  section: GQL.PersonalSettingSection;
  key: string;
}

type PersonalSetting = GQL.PersonalSettingsQuery["personalSettings"][number];

export function interfaceSetting(key: string): IPersonalSettingKey {
  return { section: GQL.PersonalSettingSection.Interface, key };
}

export function uiSetting(key: string): IPersonalSettingKey {
  return { section: GQL.PersonalSettingSection.Ui, key };
}

export function personalSettingID(s: IPersonalSettingKey) {
  return `${s.section}:${s.key}`;
}

function settingInput(s: IPersonalSettingKey): GQL.PersonalSettingInput {
  return { section: s.section, key: s.key };
}

// the UI keys holding one setting per list or table
const perListKeys: Record<string, string> = {
  tableColumns: "config.ui.personal_settings.labels.table_columns",
  defaultFilters: "config.ui.personal_settings.labels.default_filter",
  pinnedFilters: "config.ui.personal_settings.labels.pinned_filters",
  pinnedSortBy: "config.ui.personal_settings.labels.pinned_sort_by",
};

// the other list settings
const listSettingKeys: Record<string, string> = {
  frontPageContent: "config.ui.personal_settings.labels.front_page",
  taggerConfig: "config.ui.personal_settings.labels.tagger_settings",
};

// the other UI settings saved where they are changed
const otherSettingKeys: Record<string, string> = {
  advancedMode: "config.advanced_mode",
  disableAutoTagWarning: "config.tasks.auto_tag.disable_warning.heading",
};

function splitListKey(key: string) {
  const dot = key.indexOf(".");
  if (dot === -1) return;
  return { parent: key.slice(0, dot), list: key.slice(dot + 1) };
}

// returns true for the list settings, which holders of the shared list
// settings permission may publish
function isListSetting(s: IPersonalSettingKey) {
  if (s.section !== GQL.PersonalSettingSection.Ui) return false;

  const parent = splitListKey(s.key)?.parent;
  return (!!parent && !!perListKeys[parent]) || !!listSettingKeys[s.key];
}

// message ids of the headings that label the settings with a row on the
// page, by personal setting id
export type PersonalSettingHeadings = ReadonlyMap<string, string[]>;

const noHeadings: PersonalSettingHeadings = new Map();

export function personalSettingLabel(
  s: IPersonalSettingKey,
  headings: PersonalSettingHeadings,
  intl: IntlShape
) {
  const ids = headings.get(personalSettingID(s));
  if (ids) {
    return ids.map((id) => intl.formatMessage({ id })).join(": ");
  }

  if (s.section === GQL.PersonalSettingSection.Ui) {
    const listKey = splitListKey(s.key);
    const listMessage = listKey && perListKeys[listKey.parent];
    if (listKey && listMessage) {
      return intl.formatMessage({ id: listMessage }, { list: listKey.list });
    }

    const message = listSettingKeys[s.key] ?? otherSettingKeys[s.key];
    if (message) {
      return intl.formatMessage({ id: message });
    }
  }

  return s.key;
}

interface IPersonalSettingsContext {
  settings: PersonalSetting[];
  headings: PersonalSettingHeadings;
  // resets the given settings, or every personal setting if none are given
  reset: (settings?: IPersonalSettingKey[]) => Promise<void>;
}

const PersonalSettingsContext =
  React.createContext<IPersonalSettingsContext | null>(null);

export const PersonalSettingsProvider: React.FC<{
  headings?: PersonalSettingHeadings;
}> = ({ headings = noHeadings, children }) => {
  const Toast = useToast();
  const { data } = usePersonalSettings();
  const [resetSettings] = useResetPersonalSettings();
  const { reloadInterface, flushInterface } = useSettingsOptional();

  const reset = useCallback(
    async (settings?: IPersonalSettingKey[]) => {
      try {
        // a change waiting to be saved would come back after the reset
        await flushInterface();
        await resetSettings({
          variables: { settings: settings?.map(settingInput) },
        });
        // the settings page keeps its own copy of the settings
        reloadInterface();
      } catch (e) {
        Toast.error(e);
      }
    },
    [resetSettings, reloadInterface, flushInterface, Toast]
  );

  const value = useMemo(
    () => ({
      settings: data?.personalSettings ?? [],
      headings,
      reset,
    }),
    [data, headings, reset]
  );

  return (
    <PersonalSettingsContext.Provider value={value}>
      {children}
    </PersonalSettingsContext.Provider>
  );
};

export function usePersonalSetting(
  section: GQL.PersonalSettingSection,
  key: string
) {
  const context = useContext(PersonalSettingsContext);

  const isPersonal = !!context?.settings.some(
    (s) => s.section === section && s.key === key
  );

  const reset = useCallback(
    () => context?.reset([{ section, key }]),
    [context, section, key]
  );

  return { isPersonal, reset };
}

// shows that the current user changed the setting for themselves, with a
// button to reset it to the default
export const PersonalSettingBadge: React.FC<{
  setting: IPersonalSettingKey;
}> = ({ setting }) => {
  const intl = useIntl();
  const { isPersonal, reset } = usePersonalSetting(
    setting.section,
    setting.key
  );

  if (!isPersonal) return null;

  return (
    <span className="personal-setting-badge">
      <Badge variant="info">
        <FormattedMessage id="config.ui.personal_settings.own" />
      </Badge>
      <Button
        variant="minimal"
        size="sm"
        title={intl.formatMessage({ id: "config.ui.personal_settings.reset" })}
        onClick={(e) => {
          // the setting row may react to clicks
          e.stopPropagation();
          reset();
        }}
      >
        <Icon icon={faRotateLeft} />
      </Button>
    </span>
  );
};

function isScalar(v: unknown): v is string | number | boolean {
  return (
    typeof v === "string" || typeof v === "number" || typeof v === "boolean"
  );
}

function isFrontPage(s: IPersonalSettingKey) {
  return (
    s.section === GQL.PersonalSettingSection.Ui && s.key === "frontPageContent"
  );
}

// returns the names of the personal saved filters that the front page rows of
// the settings show, which the other users cannot see
function usePersonalFrontPageFilters(settings: PersonalSetting[]) {
  const { data } = useFindSavedFilters();

  return useMemo(() => {
    const value = settings.find(isFrontPage)?.value;
    const rows: FrontPageContent[] = Array.isArray(value) ? value : [];

    return rows.flatMap((r) => {
      if (r.__typename !== "SavedFilter") return [];

      const f = data?.findSavedFilters.find(
        (sf) => sf.id === String(r.savedFilterId)
      );
      return f?.personal ? [f.name] : [];
    });
  }, [settings, data]);
}

export const PublishSettingsDialog: React.FC<{
  settings: PersonalSetting[];
  onClose: () => void;
}> = ({ settings, onClose }) => {
  const intl = useIntl();
  const Toast = useToast();
  const headings = useContext(PersonalSettingsContext)?.headings ?? noHeadings;
  const [publishSettings] = usePublishPersonalSettings();
  const { flushInterface } = useSettingsOptional();
  const personalFilters = usePersonalFrontPageFilters(settings);

  const [checked, setChecked] = useState<string[]>([]);
  const [publishing, setPublishing] = useState(false);

  function toggle(id: string) {
    setChecked((current) =>
      current.includes(id) ? current.filter((c) => c !== id) : [...current, id]
    );
  }

  // the server refuses it too
  function isBlocked(s: PersonalSetting) {
    return isFrontPage(s) && personalFilters.length > 0;
  }

  const selected = settings.filter(
    (s) => !isBlocked(s) && checked.includes(personalSettingID(s))
  );

  async function onPublish() {
    setPublishing(true);
    try {
      // publish the values that wait to be saved too
      await flushInterface();
      await publishSettings({
        variables: { settings: selected.map(settingInput) },
      });
      Toast.success(
        intl.formatMessage({
          id: "config.ui.personal_settings.publish_dialog.published",
        })
      );
      onClose();
    } catch (e) {
      Toast.error(e);
      setPublishing(false);
    }
  }

  function renderValues(s: PersonalSetting) {
    const { value, defaultValue } = s;
    if (!isScalar(value)) return;
    if (defaultValue !== null && defaultValue !== undefined) {
      if (!isScalar(defaultValue)) return;
    }

    return (
      <small className="d-block text-muted">
        {intl.formatMessage(
          { id: "config.ui.personal_settings.publish_dialog.values" },
          {
            value: String(value),
            defaultValue: isScalar(defaultValue)
              ? String(defaultValue)
              : intl.formatMessage({
                  id: "config.ui.personal_settings.publish_dialog.not_set",
                }),
          }
        )}
      </small>
    );
  }

  return (
    <ModalComponent
      show
      onHide={onClose}
      dialogClassName="publish-settings-dialog"
      header={intl.formatMessage({
        id: "config.ui.personal_settings.publish_dialog.heading",
      })}
      accept={{
        text: intl.formatMessage({
          id: "config.ui.personal_settings.publish_dialog.publish",
        }),
        onClick: onPublish,
      }}
      cancel={{ variant: "secondary", onClick: onClose }}
      disabled={selected.length === 0}
      isRunning={publishing}
    >
      <p>
        <FormattedMessage id="config.ui.personal_settings.publish_dialog.description" />
      </p>
      {settings.map((s) => {
        const id = personalSettingID(s);
        const blocked = isBlocked(s);
        return (
          <Form.Check
            key={id}
            id={`publish-setting-${id}`}
            checked={!blocked && checked.includes(id)}
            disabled={blocked}
            onChange={() => toggle(id)}
            label={
              <>
                <span>{personalSettingLabel(s, headings, intl)}</span>
                {renderValues(s)}
                {blocked && (
                  <small className="d-block text-warning">
                    {intl.formatMessage(
                      {
                        id: "config.ui.personal_settings.publish_dialog.personal_filters",
                      },
                      { names: personalFilters.join(", ") }
                    )}
                  </small>
                )}
              </>
            }
          />
        );
      })}
    </ModalComponent>
  );
};

export const PersonalSettingsToolbar: React.FC = () => {
  const context = useContext(PersonalSettingsContext);
  const { isAdmin, can } = usePermissions();

  const [confirmingReset, setConfirmingReset] = useState(false);
  const [publishing, setPublishing] = useState(false);

  if (!context || context.settings.length === 0) return null;

  // admins publish every setting, holders of the shared list settings
  // permission their list settings
  const publishable = context.settings.filter(
    (s) =>
      isAdmin ||
      (isListSetting(s) && can(GQL.Permission.ManageSharedListSettings))
  );

  return (
    <div className="personal-settings-toolbar">
      <Button variant="secondary" onClick={() => setConfirmingReset(true)}>
        <FormattedMessage id="config.ui.personal_settings.reset_all" />
      </Button>
      {publishable.length > 0 && (
        <Button variant="secondary" onClick={() => setPublishing(true)}>
          <FormattedMessage id="config.ui.personal_settings.publish" />
        </Button>
      )}

      <AlertModal
        show={confirmingReset}
        text={
          <FormattedMessage id="config.ui.personal_settings.reset_all_confirm" />
        }
        onConfirm={() => {
          setConfirmingReset(false);
          context.reset();
        }}
        onCancel={() => setConfirmingReset(false)}
      />
      {publishing && (
        <PublishSettingsDialog
          settings={publishable}
          onClose={() => setPublishing(false)}
        />
      )}
    </div>
  );
};

// lists the personal settings that have no row on the page, such as the
// list settings saved in place
export const PersonalListSettings: React.FC = () => {
  const intl = useIntl();
  const context = useContext(PersonalSettingsContext);

  if (!context) return null;

  const { settings, headings, reset } = context;
  const unlisted = settings.filter((s) => !headings.has(personalSettingID(s)));

  if (unlisted.length === 0) return null;

  return (
    <SettingSection
      id="personal-list-settings"
      headingID="config.ui.personal_settings.list_settings.heading"
      subHeadingID="config.ui.personal_settings.list_settings.description"
    >
      {unlisted.map((s) => (
        <div className="setting" key={personalSettingID(s)}>
          <div>
            <h3>{personalSettingLabel(s, headings, intl)}</h3>
          </div>
          <div>
            <Button variant="secondary" onClick={() => reset([s])}>
              <Icon icon={faRotateLeft} />{" "}
              <FormattedMessage id="config.ui.personal_settings.reset" />
            </Button>
          </div>
        </div>
      ))}
    </SettingSection>
  );
};
