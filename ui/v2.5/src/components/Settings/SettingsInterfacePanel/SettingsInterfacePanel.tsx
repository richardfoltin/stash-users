import React, { useCallback, useMemo } from "react";
import { Button, Form } from "react-bootstrap";
import { FormattedMessage, useIntl } from "react-intl";
import { DurationInput } from "src/components/Shared/DurationInput";
import { PercentInput } from "src/components/Shared/PercentInput";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { CheckboxGroup } from "./CheckboxGroup";
import { SettingSection } from "../SettingSection";
import {
  BooleanSetting,
  ModalSetting,
  NumberSetting,
  SelectSetting,
  StringSetting,
} from "../Inputs";
import { useSettings } from "../context";
import TextUtils from "src/utils/text";
import * as GQL from "src/core/generated-graphql";
import {
  imageLightboxDisplayModeIntlMap,
  imageLightboxScrollModeIntlMap,
} from "src/core/enums";
import {
  ConnectionState,
  connectionStateLabel,
  InteractiveContext,
} from "src/hooks/Interactive/context";
import {
  defaultRatingStarPrecision,
  defaultRatingSystemOptions,
  defaultRatingSystemType,
  RatingStarPrecision,
  ratingStarPrecisionIntlMap,
  ratingSystemIntlMap,
  RatingSystemType,
} from "src/utils/rating";
import {
  imageWallDirectionIntlMap,
  ImageWallDirection,
  defaultImageWallOptions,
  defaultImageWallDirection,
  defaultImageWallMargin,
} from "src/utils/imageWall";
import { defaultMaxOptionsShown, defaultPreviewVolume } from "src/core/config";
import { PatchComponent } from "src/patch";
import { usePermissions } from "src/hooks/Permissions";
import {
  interfaceSetting,
  IPersonalSettingKey,
  personalSettingID,
  PersonalListSettings,
  PersonalSettingBadge,
  PersonalSettingsProvider,
  PersonalSettingsToolbar,
  uiSetting,
} from "../PersonalSettings";

const allMenuItems = [
  { id: "scenes", headingID: "scenes" },
  { id: "images", headingID: "images" },
  { id: "groups", headingID: "groups" },
  { id: "markers", headingID: "markers" },
  { id: "galleries", headingID: "galleries" },
  { id: "performers", headingID: "performers" },
  { id: "studios", headingID: "studios" },
  { id: "tags", headingID: "tags" },
];

// the personal settings that have a row on this page, with the message ids of
// the headings that label them
const rowHeadings: [IPersonalSettingKey, string[]][] = [
  [interfaceSetting("language"), ["config.ui.language.heading"]],
  [interfaceSetting("sfwContentMode"), ["config.ui.sfw_mode.heading"]],
  [interfaceSetting("menuItems"), ["config.ui.menu_items.heading"]],
  [uiSetting("abbreviateCounters"), ["config.ui.abbreviate_counters.heading"]],
  [
    interfaceSetting("soundOnPreview"),
    ["config.ui.scene_wall.options.toggle_sound"],
  ],
  [
    uiSetting("previewVolume"),
    ["config.ui.scene_view.options.preview_volume.heading"],
  ],
  [
    interfaceSetting("wallShowTitle"),
    ["config.ui.scene_wall.options.display_title"],
  ],
  [interfaceSetting("wallPlayback"), ["config.ui.preview_type.heading"]],
  [
    interfaceSetting("showStudioAsText"),
    ["config.ui.scene_list.options.show_studio_as_text"],
  ],
  [
    uiSetting("enableChromecast"),
    ["config.ui.scene_player.options.enable_chromecast"],
  ],
  [
    uiSetting("disableMobileMediaAutoRotateEnabled"),
    ["config.ui.scene_player.options.disable_mobile_media_auto_rotate"],
  ],
  [
    interfaceSetting("showScrubber"),
    ["config.ui.scene_player.options.show_scrubber"],
  ],
  [
    uiSetting("showRangeMarkers"),
    ["config.ui.scene_player.options.show_range_markers"],
  ],
  [
    uiSetting("alwaysStartFromBeginning"),
    ["config.ui.scene_player.options.always_start_from_beginning"],
  ],
  [
    uiSetting("trackActivity"),
    ["config.ui.scene_player.options.track_activity"],
  ],
  [uiSetting("vrTag"), ["config.ui.scene_player.options.vr_tag.heading"]],
  [uiSetting("minimumPlayPercent"), ["config.ui.minimum_play_percent.heading"]],
  [
    interfaceSetting("imageLightbox.slideshowDelay"),
    ["config.ui.slideshow_delay.heading"],
  ],
  [
    interfaceSetting("autostartVideo"),
    ["config.ui.scene_player.options.auto_start_video"],
  ],
  [
    interfaceSetting("autostartVideoOnPlaySelected"),
    [
      "config.ui.scene_player.options.auto_start_video_on_play_selected.heading",
    ],
  ],
  [
    interfaceSetting("continuePlaylistDefault"),
    ["config.ui.scene_player.options.continue_playlist_default.heading"],
  ],
  [
    interfaceSetting("maximumLoopDuration"),
    ["config.ui.max_loop_duration.heading"],
  ],
  [
    uiSetting("showAbLoopControls"),
    ["config.ui.scene_player.options.show_ab_loop_controls"],
  ],
  [
    uiSetting("showTagCardOnHover"),
    ["config.ui.show_tag_card_on_hover.heading"],
  ],
  [
    uiSetting("showChildTagContent"),
    ["config.ui.tag_panel.options.show_child_tagged_content.heading"],
  ],
  [
    uiSetting("showChildStudioContent"),
    ["config.ui.studio_panel.options.show_child_studio_content.heading"],
  ],
  [
    uiSetting("showLinksOnPerformerCard"),
    ["config.ui.performer_list.options.show_links_on_grid_card.heading"],
  ],
  [uiSetting("imageWallOptions"), ["config.ui.image_wall.heading"]],
  [
    uiSetting("autostartGallerySlideshow"),
    ["config.ui.autostart_gallery_slideshow.heading"],
  ],
  [
    interfaceSetting("imageLightbox.displayMode"),
    ["dialogs.lightbox.display_mode.label"],
  ],
  [
    interfaceSetting("imageLightbox.scaleUp"),
    ["dialogs.lightbox.scale_up.label"],
  ],
  [
    interfaceSetting("imageLightbox.resetZoomOnNav"),
    ["dialogs.lightbox.reset_zoom_on_nav"],
  ],
  [
    interfaceSetting("imageLightbox.scrollMode"),
    ["dialogs.lightbox.scroll_mode.label"],
  ],
  [
    interfaceSetting("imageLightbox.scrollAttemptsBeforeChange"),
    ["config.ui.scroll_attempts_before_change.heading"],
  ],
  [
    interfaceSetting("imageLightbox.disableAnimation"),
    ["dialogs.lightbox.disable_animation"],
  ],
  [
    uiSetting("enableMovieBackgroundImage"),
    ["config.ui.detail.enable_background_image.heading", "group"],
  ],
  [
    uiSetting("enablePerformerBackgroundImage"),
    ["config.ui.detail.enable_background_image.heading", "performer"],
  ],
  [
    uiSetting("enableStudioBackgroundImage"),
    ["config.ui.detail.enable_background_image.heading", "studio"],
  ],
  [
    uiSetting("enableTagBackgroundImage"),
    ["config.ui.detail.enable_background_image.heading", "tag"],
  ],
  [uiSetting("showAllDetails"), ["config.ui.detail.show_all_details.heading"]],
  [
    uiSetting("compactExpandedDetails"),
    ["config.ui.detail.compact_expanded_details.heading"],
  ],
  [uiSetting("showStudioText"), ["config.ui.detail.show_studio_text.heading"]],
  [
    interfaceSetting("disableDropdownCreate.performer"),
    ["config.ui.editing.disable_dropdown_create.heading", "performer"],
  ],
  [
    interfaceSetting("disableDropdownCreate.studio"),
    ["config.ui.editing.disable_dropdown_create.heading", "studio"],
  ],
  [
    interfaceSetting("disableDropdownCreate.tag"),
    ["config.ui.editing.disable_dropdown_create.heading", "tag"],
  ],
  [
    interfaceSetting("disableDropdownCreate.movie"),
    ["config.ui.editing.disable_dropdown_create.heading", "group"],
  ],
  [
    interfaceSetting("disableDropdownCreate.gallery"),
    ["config.ui.editing.disable_dropdown_create.heading", "gallery"],
  ],
  [uiSetting("maxOptionsShown"), ["config.ui.editing.max_options_shown.label"]],
  [
    uiSetting("ratingSystemOptions"),
    ["config.ui.editing.rating_system.type.label"],
  ],
];

// the ids of the personal settings that have a row on this page, with the
// headings that label them
export const interfacePanelSettings: ReadonlyMap<string, string[]> = new Map(
  rowHeadings.map(([setting, headings]) => [
    personalSettingID(setting),
    headings,
  ])
);

export const SettingsInterfacePanel: React.FC = PatchComponent(
  "SettingsInterfacePanel",
  function SettingsInterfacePanel() {
    const intl = useIntl();
    // the shared settings apply to every user, and only admins change them
    const { isAdmin } = usePermissions();

    const {
      interface: iface,
      saveInterface,
      ui,
      saveUI,
      loading,
      error,
    } = useSettings();
    // convert old movies menu item to groups
    const massageMenuItems = useCallback((menuItems: string[]) => {
      return menuItems.map((item) => {
        if (item === "movies") {
          return "groups";
        }
        return item;
      });
    }, []);

    const massagedMenuItems = useMemo(() => {
      if (!iface.menuItems) return iface.menuItems;

      return massageMenuItems(iface.menuItems);
    }, [iface.menuItems, massageMenuItems]);

    const {
      interactive,
      state: interactiveState,
      error: interactiveError,
      serverOffset: interactiveServerOffset,
      initialised: interactiveInitialised,
      initialise: initialiseInteractive,
      sync: interactiveSync,
    } = React.useContext(InteractiveContext);

    function saveLightboxSettings(v: Partial<GQL.ConfigImageLightboxInput>) {
      saveInterface({ imageLightbox: v });
    }

    function saveImageWallMargin(m: number) {
      saveUI({
        imageWallOptions: {
          ...(ui.imageWallOptions ?? defaultImageWallOptions),
          margin: m,
        },
      });
    }

    function saveImageWallDirection(d: ImageWallDirection) {
      saveUI({
        imageWallOptions: {
          ...(ui.imageWallOptions ?? defaultImageWallOptions),
          direction: d,
        },
      });
    }

    function saveRatingSystemType(t: RatingSystemType) {
      saveUI({
        ratingSystemOptions: {
          ...ui.ratingSystemOptions,
          type: t,
        },
      });
    }

    function saveRatingSystemStarPrecision(p: RatingStarPrecision) {
      saveUI({
        ratingSystemOptions: {
          ...(ui.ratingSystemOptions ?? defaultRatingSystemOptions),
          starPrecision: p,
        },
      });
    }

    function validateLocaleString(v: string) {
      if (!v) return;
      try {
        JSON.parse(v);
      } catch (e) {
        throw new Error(
          intl.formatMessage(
            { id: "errors.invalid_json_string" },
            {
              error: (e as SyntaxError).message,
            }
          )
        );
      }
    }

    function validateJavascriptString(v: string) {
      if (!v) return;
      try {
        // creates a function from the string to validate it but does not execute it
        // XXbiome-ignore lint/nursery/noImpliedEval: intentional
        new Function(v);
      } catch (e) {
        throw new Error(
          intl.formatMessage(
            { id: "errors.invalid_javascript_string" },
            {
              error: (e as SyntaxError).message,
            }
          )
        );
      }
    }

    if (error) return <h1>{error.message}</h1>;
    if (loading) return <LoadingIndicator />;

    // https://en.wikipedia.org/wiki/List_of_language_names

    return (
      <PersonalSettingsProvider headings={interfacePanelSettings}>
        <PersonalSettingsToolbar />

        <SettingSection headingID="config.ui.basic_settings">
          <SelectSetting
            id="language"
            personal={interfaceSetting("language")}
            headingID="config.ui.language.heading"
            value={iface.language ?? undefined}
            onChange={(v) => saveInterface({ language: v })}
          >
            <option value="af-ZA">Afrikaans (Preview)</option>
            <option value="ar">Arabic (Preview)</option>
            <option value="bg-BG">Bulgarian (Preview)</option>
            <option value="bn-BD">বাংলা (বাংলাদেশ) (Preview)</option>
            <option value="ca-ES">Catalan (Preview)</option>
            <option value="cs-CZ">Čeština (Česko)</option>
            <option value="da-DK">Dansk (Danmark)</option>
            <option value="de-DE">Deutsch (Deutschland)</option>
            <option value="en-GB">English (United Kingdom)</option>
            <option value="en-US">English (United States)</option>
            <option value="et-EE">Eesti</option>
            <option value="fa-IR">فارسی (ایران) (Preview)</option>
            <option value="fi-FI">Suomi</option>
            <option value="fr-FR">Français (France)</option>
            <option value="hi-IN">हिन्दी (Preview)</option>
            <option value="hr-HR">Hrvatski (Preview)</option>
            <option value="id-ID">Indonesian (Preview)</option>
            <option value="hu-HU">Magyar (Preview)</option>
            <option value="it-IT">Italiano</option>
            <option value="ja-JP">日本語 (日本)</option>
            <option value="ko-KR">한국어 (대한민국)</option>
            <option value="lv-LV">Latviešu (Preview)</option>
            <option value="lt-LT">Lithuanian (Preview)</option>
            <option value="nb-NO">Norsk bokmål</option>
            <option value="nn-NO">Nynorsk (Preview)</option>
            <option value="nl-NL">Nederlands (Nederland)</option>
            <option value="pl-PL">Polski</option>
            <option value="pt-BR">Português (Brasil)</option>
            <option value="ro-RO">Română (Preview)</option>
            <option value="ru-RU">Русский (Россия)</option>
            <option value="es-ES">Español (España)</option>
            <option value="sk-SK">Slovenčina (Preview)</option>
            <option value="sv-SE">Svenska</option>
            <option value="tr-TR">Türkçe (Türkiye)</option>
            <option value="th-TH">ภาษาไทย (ไทย)</option>
            <option value="uk-UA">Ukrainian (Україна)</option>
            <option value="ur-PK">Urdu (Preview)</option>
            <option value="vi-VN">Tiếng Việt (Preview)</option>
            <option value="zh-TW">繁體中文 (台灣)</option>
            <option value="zh-CN">简体中文 (中国)</option>
          </SelectSetting>

          <BooleanSetting
            id="sfw-content-mode"
            personal={interfaceSetting("sfwContentMode")}
            headingID="config.ui.sfw_mode.heading"
            subHeadingID="config.ui.sfw_mode.description"
            checked={iface.sfwContentMode ?? undefined}
            onChange={(v) => saveInterface({ sfwContentMode: v })}
          />

          {isAdmin && (
            <StringSetting
              id="custom-title"
              headingID="config.ui.custom_title.heading"
              subHeading={
                <>
                  <div>
                    {intl.formatMessage({
                      id: "config.ui.custom_title.description",
                    })}
                  </div>
                  <div>
                    {intl.formatMessage({
                      id: "config.ui.shared_setting_note",
                    })}
                  </div>
                </>
              }
              value={ui.title ?? ""}
              onChange={(v) => saveUI({ title: v })}
            />
          )}

          <div className="setting-group">
            <div className="setting">
              <div>
                <h3>
                  {intl.formatMessage({
                    id: "config.ui.menu_items.heading",
                  })}
                  <PersonalSettingBadge
                    setting={interfaceSetting("menuItems")}
                  />
                </h3>
                <div className="sub-heading">
                  {intl.formatMessage({
                    id: "config.ui.menu_items.description",
                  })}
                </div>
              </div>
              <div />
            </div>
            <CheckboxGroup
              groupId="menu-items"
              items={allMenuItems}
              checkedIds={massagedMenuItems ?? undefined}
              onChange={(v) =>
                saveInterface({ menuItems: massageMenuItems(v) })
              }
            />
          </div>

          <BooleanSetting
            id="abbreviate-counters"
            personal={uiSetting("abbreviateCounters")}
            headingID="config.ui.abbreviate_counters.heading"
            subHeadingID="config.ui.abbreviate_counters.description"
            checked={ui.abbreviateCounters ?? undefined}
            onChange={(v) => saveUI({ abbreviateCounters: v })}
          />
        </SettingSection>

        {isAdmin && (
          <SettingSection
            headingID="config.ui.desktop_integration.desktop_integration"
            subHeadingID="config.ui.shared_setting_note"
          >
            <BooleanSetting
              id="skip-browser"
              headingID="config.ui.desktop_integration.skip_opening_browser"
              subHeadingID="config.ui.desktop_integration.skip_opening_browser_on_startup"
              checked={iface.noBrowser ?? undefined}
              onChange={(v) => saveInterface({ noBrowser: v })}
            />
            <BooleanSetting
              id="notifications-enabled"
              headingID="config.ui.desktop_integration.notifications_enabled"
              subHeadingID="config.ui.desktop_integration.send_desktop_notifications_for_events"
              checked={iface.notificationsEnabled ?? undefined}
              onChange={(v) => saveInterface({ notificationsEnabled: v })}
            />
          </SettingSection>
        )}

        <SettingSection headingID="config.ui.scene_view.heading">
          <BooleanSetting
            id="sound-on-hover"
            personal={interfaceSetting("soundOnPreview")}
            headingID="config.ui.scene_wall.options.toggle_sound"
            checked={iface.soundOnPreview ?? undefined}
            onChange={(v) => saveInterface({ soundOnPreview: v })}
          />
          <ModalSetting<number>
            id="preview-volume"
            personal={uiSetting("previewVolume")}
            headingID="config.ui.scene_view.options.preview_volume.heading"
            subHeadingID="config.ui.scene_view.options.preview_volume.description"
            value={ui.previewVolume ?? defaultPreviewVolume}
            onChange={(v) => saveUI({ previewVolume: v })}
            disabled={!iface.soundOnPreview}
            renderField={(value, setValue) => (
              <PercentInput
                numericValue={value}
                onValueChange={(v) => setValue(v ?? 0)}
              />
            )}
            renderValue={(v) => {
              return <span>{v}%</span>;
            }}
          />
        </SettingSection>

        <SettingSection headingID="config.ui.scene_wall.heading">
          <BooleanSetting
            id="wall-show-title"
            personal={interfaceSetting("wallShowTitle")}
            headingID="config.ui.scene_wall.options.display_title"
            checked={iface.wallShowTitle ?? undefined}
            onChange={(v) => saveInterface({ wallShowTitle: v })}
          />
          <SelectSetting
            advanced
            id="wall-preview"
            personal={interfaceSetting("wallPlayback")}
            headingID="config.ui.preview_type.heading"
            subHeadingID="config.ui.preview_type.description"
            value={iface.wallPlayback ?? undefined}
            onChange={(v) => saveInterface({ wallPlayback: v })}
          >
            <option value="video">
              {intl.formatMessage({
                id: "config.ui.preview_type.options.video",
              })}
            </option>
            <option value="animation">
              {intl.formatMessage({
                id: "config.ui.preview_type.options.animated",
              })}
            </option>
            <option value="image">
              {intl.formatMessage({
                id: "config.ui.preview_type.options.static",
              })}
            </option>
          </SelectSetting>
        </SettingSection>

        <SettingSection headingID="config.ui.scene_list.heading">
          <BooleanSetting
            id="show-text-studios"
            personal={interfaceSetting("showStudioAsText")}
            headingID="config.ui.scene_list.options.show_studio_as_text"
            subHeadingID="config.ui.scene_list.options.show_studio_as_text_desc"
            checked={iface.showStudioAsText ?? undefined}
            onChange={(v) => saveInterface({ showStudioAsText: v })}
          />
        </SettingSection>

        <SettingSection headingID="config.ui.scene_player.heading">
          <BooleanSetting
            id="enable-chromecast"
            personal={uiSetting("enableChromecast")}
            headingID="config.ui.scene_player.options.enable_chromecast"
            checked={ui.enableChromecast ?? undefined}
            onChange={(v) => saveUI({ enableChromecast: v })}
          />
          <BooleanSetting
            id="disable-mobile-media-auto-rotate"
            personal={uiSetting("disableMobileMediaAutoRotateEnabled")}
            headingID="config.ui.scene_player.options.disable_mobile_media_auto_rotate"
            checked={ui.disableMobileMediaAutoRotateEnabled ?? undefined}
            onChange={(v) => saveUI({ disableMobileMediaAutoRotateEnabled: v })}
          />
          <BooleanSetting
            id="show-scrubber"
            personal={interfaceSetting("showScrubber")}
            headingID="config.ui.scene_player.options.show_scrubber"
            checked={iface.showScrubber ?? undefined}
            onChange={(v) => saveInterface({ showScrubber: v })}
          />
          <BooleanSetting
            id="show-range-markers"
            personal={uiSetting("showRangeMarkers")}
            headingID="config.ui.scene_player.options.show_range_markers"
            checked={ui.showRangeMarkers ?? undefined}
            onChange={(v) => saveUI({ showRangeMarkers: v })}
          />
          <BooleanSetting
            id="always-start-from-beginning"
            personal={uiSetting("alwaysStartFromBeginning")}
            headingID="config.ui.scene_player.options.always_start_from_beginning"
            checked={ui.alwaysStartFromBeginning ?? undefined}
            onChange={(v) => saveUI({ alwaysStartFromBeginning: v })}
          />
          <BooleanSetting
            id="track-activity"
            personal={uiSetting("trackActivity")}
            headingID="config.ui.scene_player.options.track_activity"
            checked={ui.trackActivity ?? true}
            onChange={(v) => saveUI({ trackActivity: v })}
          />
          <StringSetting
            id="vr-tag"
            personal={uiSetting("vrTag")}
            headingID="config.ui.scene_player.options.vr_tag.heading"
            subHeadingID="config.ui.scene_player.options.vr_tag.description"
            value={ui.vrTag ?? undefined}
            onChange={(v) => saveUI({ vrTag: v })}
          />
          <ModalSetting<number>
            id="ignore-interval"
            personal={uiSetting("minimumPlayPercent")}
            headingID="config.ui.minimum_play_percent.heading"
            subHeadingID="config.ui.minimum_play_percent.description"
            value={ui.minimumPlayPercent ?? 0}
            onChange={(v) => saveUI({ minimumPlayPercent: v })}
            disabled={!ui.trackActivity}
            renderField={(value, setValue) => (
              <PercentInput
                numericValue={value}
                onValueChange={(interval) => setValue(interval ?? 0)}
              />
            )}
            renderValue={(v) => {
              return <span>{v}%</span>;
            }}
          />
          <NumberSetting
            headingID="config.ui.slideshow_delay.heading"
            personal={interfaceSetting("imageLightbox.slideshowDelay")}
            subHeadingID="config.ui.slideshow_delay.description"
            value={iface.imageLightbox?.slideshowDelay ?? undefined}
            onChange={(v) => saveLightboxSettings({ slideshowDelay: v })}
          />
          <BooleanSetting
            id="auto-start-video"
            personal={interfaceSetting("autostartVideo")}
            headingID="config.ui.scene_player.options.auto_start_video"
            checked={iface.autostartVideo ?? undefined}
            onChange={(v) => saveInterface({ autostartVideo: v })}
          />
          <BooleanSetting
            id="auto-start-video-on-play-selected"
            personal={interfaceSetting("autostartVideoOnPlaySelected")}
            headingID="config.ui.scene_player.options.auto_start_video_on_play_selected.heading"
            subHeadingID="config.ui.scene_player.options.auto_start_video_on_play_selected.description"
            checked={iface.autostartVideoOnPlaySelected ?? undefined}
            onChange={(v) => saveInterface({ autostartVideoOnPlaySelected: v })}
          />

          <BooleanSetting
            id="continue-playlist-default"
            personal={interfaceSetting("continuePlaylistDefault")}
            headingID="config.ui.scene_player.options.continue_playlist_default.heading"
            subHeadingID="config.ui.scene_player.options.continue_playlist_default.description"
            checked={iface.continuePlaylistDefault ?? undefined}
            onChange={(v) => saveInterface({ continuePlaylistDefault: v })}
          />

          <ModalSetting<number>
            id="max-loop-duration"
            personal={interfaceSetting("maximumLoopDuration")}
            headingID="config.ui.max_loop_duration.heading"
            subHeadingID="config.ui.max_loop_duration.description"
            value={iface.maximumLoopDuration ?? undefined}
            onChange={(v) => saveInterface({ maximumLoopDuration: v })}
            renderField={(value, setValue) => (
              <DurationInput
                value={value}
                setValue={(duration) => setValue(duration ?? 0)}
              />
            )}
            renderValue={(v) => {
              return <span>{TextUtils.secondsToTimestamp(v ?? 0)}</span>;
            }}
          />

          <BooleanSetting
            id="show-ab-loop"
            personal={uiSetting("showAbLoopControls")}
            headingID="config.ui.scene_player.options.show_ab_loop_controls"
            checked={ui.showAbLoopControls ?? undefined}
            onChange={(v) => saveUI({ showAbLoopControls: v })}
          />
        </SettingSection>
        <SettingSection headingID="config.ui.tag_panel.heading">
          <BooleanSetting
            id="show-tag-card-on-hover"
            personal={uiSetting("showTagCardOnHover")}
            headingID="config.ui.show_tag_card_on_hover.heading"
            subHeadingID="config.ui.show_tag_card_on_hover.description"
            checked={ui.showTagCardOnHover ?? true}
            onChange={(v) => saveUI({ showTagCardOnHover: v })}
          />
          <BooleanSetting
            id="show-child-tagged-content"
            personal={uiSetting("showChildTagContent")}
            headingID="config.ui.tag_panel.options.show_child_tagged_content.heading"
            subHeadingID="config.ui.tag_panel.options.show_child_tagged_content.description"
            checked={ui.showChildTagContent ?? undefined}
            onChange={(v) => saveUI({ showChildTagContent: v })}
          />
        </SettingSection>

        <SettingSection headingID="config.ui.studio_panel.heading">
          <BooleanSetting
            id="show-child-studio-content"
            personal={uiSetting("showChildStudioContent")}
            headingID="config.ui.studio_panel.options.show_child_studio_content.heading"
            subHeadingID="config.ui.studio_panel.options.show_child_studio_content.description"
            checked={ui.showChildStudioContent ?? undefined}
            onChange={(v) => saveUI({ showChildStudioContent: v })}
          />
        </SettingSection>

        <SettingSection headingID="config.ui.performer_list.heading">
          <BooleanSetting
            id="show-links-on-grid-card"
            personal={uiSetting("showLinksOnPerformerCard")}
            headingID="config.ui.performer_list.options.show_links_on_grid_card.heading"
            checked={ui.showLinksOnPerformerCard ?? undefined}
            onChange={(v) => saveUI({ showLinksOnPerformerCard: v })}
          />
        </SettingSection>

        <SettingSection headingID="config.ui.image_wall.heading">
          <NumberSetting
            headingID="config.ui.image_wall.margin"
            personal={uiSetting("imageWallOptions")}
            subHeadingID="dialogs.imagewall.margin_desc"
            value={ui.imageWallOptions?.margin ?? defaultImageWallMargin}
            onChange={(v) => saveImageWallMargin(v)}
          />

          <SelectSetting
            id="image_wall_direction"
            personal={uiSetting("imageWallOptions")}
            headingID="config.ui.image_wall.direction"
            subHeadingID="dialogs.imagewall.direction.description"
            value={ui.imageWallOptions?.direction ?? defaultImageWallDirection}
            onChange={(v) => saveImageWallDirection(v as ImageWallDirection)}
          >
            {Array.from(imageWallDirectionIntlMap.entries()).map((v) => (
              <option key={v[0]} value={v[0]}>
                {intl.formatMessage({
                  id: v[1],
                })}
              </option>
            ))}
          </SelectSetting>
        </SettingSection>

        <SettingSection headingID="config.ui.image_lightbox.heading">
          <NumberSetting
            headingID="config.ui.slideshow_delay.heading"
            personal={interfaceSetting("imageLightbox.slideshowDelay")}
            subHeadingID="config.ui.slideshow_delay.description"
            value={iface.imageLightbox?.slideshowDelay ?? undefined}
            onChange={(v) => saveLightboxSettings({ slideshowDelay: v })}
          />

          <BooleanSetting
            id="autostart-gallery-slideshow"
            personal={uiSetting("autostartGallerySlideshow")}
            headingID="config.ui.autostart_gallery_slideshow.heading"
            subHeadingID="config.ui.autostart_gallery_slideshow.description"
            checked={ui.autostartGallerySlideshow ?? undefined}
            onChange={(v) => saveUI({ autostartGallerySlideshow: v })}
          />

          <SelectSetting
            id="lightbox_display_mode"
            personal={interfaceSetting("imageLightbox.displayMode")}
            headingID="dialogs.lightbox.display_mode.label"
            value={
              iface.imageLightbox?.displayMode ??
              GQL.ImageLightboxDisplayMode.FitXy
            }
            onChange={(v) =>
              saveLightboxSettings({
                displayMode: v as GQL.ImageLightboxDisplayMode,
              })
            }
          >
            {Array.from(imageLightboxDisplayModeIntlMap.entries()).map((v) => (
              <option key={v[0]} value={v[0]}>
                {intl.formatMessage({
                  id: v[1],
                })}
              </option>
            ))}
          </SelectSetting>

          <BooleanSetting
            id="lightbox_scale_up"
            personal={interfaceSetting("imageLightbox.scaleUp")}
            headingID="dialogs.lightbox.scale_up.label"
            subHeadingID="dialogs.lightbox.scale_up.description"
            checked={iface.imageLightbox?.scaleUp ?? false}
            onChange={(v) => saveLightboxSettings({ scaleUp: v })}
          />

          <BooleanSetting
            id="lightbox_reset_zoom_on_nav"
            personal={interfaceSetting("imageLightbox.resetZoomOnNav")}
            headingID="dialogs.lightbox.reset_zoom_on_nav"
            checked={iface.imageLightbox?.resetZoomOnNav ?? false}
            onChange={(v) => saveLightboxSettings({ resetZoomOnNav: v })}
          />

          <SelectSetting
            id="lightbox_scroll_mode"
            personal={interfaceSetting("imageLightbox.scrollMode")}
            headingID="dialogs.lightbox.scroll_mode.label"
            subHeadingID="dialogs.lightbox.scroll_mode.description"
            value={
              iface.imageLightbox?.scrollMode ??
              GQL.ImageLightboxScrollMode.Zoom
            }
            onChange={(v) =>
              saveLightboxSettings({
                scrollMode: v as GQL.ImageLightboxScrollMode,
              })
            }
          >
            {Array.from(imageLightboxScrollModeIntlMap.entries()).map((v) => (
              <option key={v[0]} value={v[0]}>
                {intl.formatMessage({
                  id: v[1],
                })}
              </option>
            ))}
          </SelectSetting>

          <NumberSetting
            headingID="config.ui.scroll_attempts_before_change.heading"
            personal={interfaceSetting(
              "imageLightbox.scrollAttemptsBeforeChange"
            )}
            subHeadingID="config.ui.scroll_attempts_before_change.description"
            value={iface.imageLightbox?.scrollAttemptsBeforeChange ?? 0}
            onChange={(v) =>
              saveLightboxSettings({ scrollAttemptsBeforeChange: v })
            }
          />

          <BooleanSetting
            id="lightbox_disable_animation"
            personal={interfaceSetting("imageLightbox.disableAnimation")}
            headingID="dialogs.lightbox.disable_animation"
            checked={iface.imageLightbox?.disableAnimation ?? false}
            onChange={(v) => saveLightboxSettings({ disableAnimation: v })}
          />
        </SettingSection>

        <SettingSection headingID="config.ui.detail.heading">
          <div className="setting-group">
            <div className="setting">
              <div>
                <h3>
                  {intl.formatMessage({
                    id: "config.ui.detail.enable_background_image.heading",
                  })}
                </h3>
                <div className="sub-heading">
                  {intl.formatMessage({
                    id: "config.ui.detail.enable_background_image.description",
                  })}
                </div>
              </div>
              <div />
            </div>
            <BooleanSetting
              id="enableMovieBackgroundImage"
              personal={uiSetting("enableMovieBackgroundImage")}
              headingID="group"
              checked={ui.enableMovieBackgroundImage ?? undefined}
              onChange={(v) => saveUI({ enableMovieBackgroundImage: v })}
            />
            <BooleanSetting
              id="enablePerformerBackgroundImage"
              personal={uiSetting("enablePerformerBackgroundImage")}
              headingID="performer"
              checked={ui.enablePerformerBackgroundImage ?? undefined}
              onChange={(v) => saveUI({ enablePerformerBackgroundImage: v })}
            />
            <BooleanSetting
              id="enableStudioBackgroundImage"
              personal={uiSetting("enableStudioBackgroundImage")}
              headingID="studio"
              checked={ui.enableStudioBackgroundImage ?? undefined}
              onChange={(v) => saveUI({ enableStudioBackgroundImage: v })}
            />
            <BooleanSetting
              id="enableTagBackgroundImage"
              personal={uiSetting("enableTagBackgroundImage")}
              headingID="tag"
              checked={ui.enableTagBackgroundImage ?? undefined}
              onChange={(v) => saveUI({ enableTagBackgroundImage: v })}
            />
          </div>
          <BooleanSetting
            id="show_all_details"
            personal={uiSetting("showAllDetails")}
            headingID="config.ui.detail.show_all_details.heading"
            subHeadingID="config.ui.detail.show_all_details.description"
            checked={ui.showAllDetails ?? true}
            onChange={(v) => saveUI({ showAllDetails: v })}
          />
          <BooleanSetting
            id="compact_expanded_details"
            personal={uiSetting("compactExpandedDetails")}
            headingID="config.ui.detail.compact_expanded_details.heading"
            subHeadingID="config.ui.detail.compact_expanded_details.description"
            checked={ui.compactExpandedDetails ?? undefined}
            onChange={(v) => saveUI({ compactExpandedDetails: v })}
          />
          <BooleanSetting
            id="show_studio_text"
            personal={uiSetting("showStudioText")}
            headingID="config.ui.detail.show_studio_text.heading"
            subHeadingID="config.ui.detail.show_studio_text.description"
            checked={ui.showStudioText ?? false}
            onChange={(v) => saveUI({ showStudioText: v })}
          />
        </SettingSection>

        <SettingSection headingID="config.ui.editing.heading">
          <div className="setting-group">
            <div className="setting">
              <div>
                <h3>
                  {intl.formatMessage({
                    id: "config.ui.editing.disable_dropdown_create.heading",
                  })}
                </h3>
                <div className="sub-heading">
                  {intl.formatMessage({
                    id: "config.ui.editing.disable_dropdown_create.description",
                  })}
                </div>
              </div>
              <div />
            </div>
            <BooleanSetting
              id="disableDropdownCreate_performer"
              personal={interfaceSetting("disableDropdownCreate.performer")}
              headingID="performer"
              checked={iface.disableDropdownCreate?.performer ?? undefined}
              onChange={(v) =>
                saveInterface({
                  disableDropdownCreate: {
                    performer: v,
                  },
                })
              }
            />
            <BooleanSetting
              id="disableDropdownCreate_studio"
              personal={interfaceSetting("disableDropdownCreate.studio")}
              headingID="studio"
              checked={iface.disableDropdownCreate?.studio ?? undefined}
              onChange={(v) =>
                saveInterface({
                  disableDropdownCreate: {
                    studio: v,
                  },
                })
              }
            />
            <BooleanSetting
              id="disableDropdownCreate_tag"
              personal={interfaceSetting("disableDropdownCreate.tag")}
              headingID="tag"
              checked={iface.disableDropdownCreate?.tag ?? undefined}
              onChange={(v) =>
                saveInterface({
                  disableDropdownCreate: {
                    tag: v,
                  },
                })
              }
            />
            <BooleanSetting
              id="disableDropdownCreate_group"
              personal={interfaceSetting("disableDropdownCreate.movie")}
              headingID="group"
              checked={iface.disableDropdownCreate?.movie ?? undefined}
              onChange={(v) =>
                saveInterface({
                  disableDropdownCreate: {
                    movie: v,
                  },
                })
              }
            />
            <BooleanSetting
              id="disableDropdownCreate_gallery"
              personal={interfaceSetting("disableDropdownCreate.gallery")}
              headingID="gallery"
              checked={iface.disableDropdownCreate?.gallery ?? undefined}
              onChange={(v) =>
                saveInterface({
                  disableDropdownCreate: {
                    gallery: v,
                  },
                })
              }
            />
          </div>
          <NumberSetting
            id="max_options_shown"
            personal={uiSetting("maxOptionsShown")}
            headingID="config.ui.editing.max_options_shown.label"
            value={ui.maxOptionsShown ?? defaultMaxOptionsShown}
            onChange={(v) => saveUI({ maxOptionsShown: v })}
          />
          <SelectSetting
            id="rating_system"
            personal={uiSetting("ratingSystemOptions")}
            headingID="config.ui.editing.rating_system.type.label"
            value={ui.ratingSystemOptions?.type ?? defaultRatingSystemType}
            onChange={(v) => saveRatingSystemType(v as RatingSystemType)}
          >
            {Array.from(ratingSystemIntlMap.entries()).map((v) => (
              <option key={v[0]} value={v[0]}>
                {intl.formatMessage({
                  id: v[1],
                })}
              </option>
            ))}
          </SelectSetting>
          {(ui.ratingSystemOptions?.type ?? defaultRatingSystemType) ===
            RatingSystemType.Stars && (
            <SelectSetting
              id="rating_system_star_precision"
              personal={uiSetting("ratingSystemOptions")}
              headingID="config.ui.editing.rating_system.star_precision.label"
              value={
                ui.ratingSystemOptions?.starPrecision ??
                defaultRatingStarPrecision
              }
              onChange={(v) =>
                saveRatingSystemStarPrecision(v as RatingStarPrecision)
              }
            >
              {Array.from(ratingStarPrecisionIntlMap.entries()).map((v) => (
                <option key={v[0]} value={v[0]}>
                  {intl.formatMessage({
                    id: v[1],
                  })}
                </option>
              ))}
            </SelectSetting>
          )}
        </SettingSection>

        {isAdmin && (
          <>
            <SettingSection
              headingID="config.ui.custom_css.heading"
              subHeadingID="config.ui.shared_setting_note"
            >
              <BooleanSetting
                id="custom-css-enabled"
                headingID="config.ui.custom_css.option_label"
                checked={iface.cssEnabled ?? undefined}
                onChange={(v) => saveInterface({ cssEnabled: v })}
              />

              <ModalSetting<string>
                id="custom-css"
                headingID="config.ui.custom_css.heading"
                subHeadingID="config.ui.custom_css.description"
                value={iface.css ?? undefined}
                onChange={(v) => saveInterface({ css: v })}
                renderField={(value, setValue) => (
                  <Form.Control
                    as="textarea"
                    value={value}
                    onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
                      setValue(e.currentTarget.value)
                    }
                    rows={16}
                    className="text-input code"
                  />
                )}
                renderValue={() => {
                  return null;
                }}
              />
            </SettingSection>
            <SettingSection
              headingID="config.ui.custom_javascript.heading"
              subHeadingID="config.ui.shared_setting_note"
            >
              <BooleanSetting
                id="custom-javascript-enabled"
                headingID="config.ui.custom_javascript.option_label"
                checked={iface.javascriptEnabled ?? undefined}
                onChange={(v) => saveInterface({ javascriptEnabled: v })}
              />

              <ModalSetting<string>
                id="custom-javascript"
                headingID="config.ui.custom_javascript.heading"
                subHeadingID="config.ui.custom_javascript.description"
                value={iface.javascript ?? undefined}
                onChange={(v) => saveInterface({ javascript: v })}
                validateChange={validateJavascriptString}
                renderField={(value, setValue, err) => (
                  <>
                    <Form.Control
                      as="textarea"
                      value={value}
                      onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
                        setValue(e.currentTarget.value)
                      }
                      rows={16}
                      className="text-input code"
                      isInvalid={!!err}
                    />
                    <Form.Control.Feedback type="invalid">
                      {err}
                    </Form.Control.Feedback>
                  </>
                )}
                renderValue={() => {
                  return null;
                }}
              />
            </SettingSection>
            <SettingSection
              headingID="config.ui.custom_locales.heading"
              subHeadingID="config.ui.shared_setting_note"
            >
              <BooleanSetting
                id="custom-locales-enabled"
                headingID="config.ui.custom_locales.option_label"
                checked={iface.customLocalesEnabled ?? undefined}
                onChange={(v) => saveInterface({ customLocalesEnabled: v })}
              />

              <ModalSetting<string>
                id="custom-locales"
                headingID="config.ui.custom_locales.heading"
                subHeadingID="config.ui.custom_locales.description"
                value={iface.customLocales ?? undefined}
                onChange={(v) => saveInterface({ customLocales: v })}
                validateChange={validateLocaleString}
                renderField={(value, setValue, err) => (
                  <>
                    <Form.Control
                      as="textarea"
                      value={value}
                      onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
                        setValue(e.currentTarget.value)
                      }
                      rows={16}
                      className="text-input code"
                      isInvalid={!!err}
                    />
                    <Form.Control.Feedback type="invalid">
                      {err}
                    </Form.Control.Feedback>
                  </>
                )}
                renderValue={() => {
                  return null;
                }}
              />
            </SettingSection>

            <SettingSection
              headingID="config.ui.interactive_options"
              subHeadingID="config.ui.shared_setting_note"
            >
              <StringSetting
                headingID="config.ui.handy_connection_key.heading"
                subHeadingID="config.ui.handy_connection_key.description"
                value={iface.handyKey ?? undefined}
                onChange={(v) => saveInterface({ handyKey: v })}
              />
              {interactive.handyKey && (
                <>
                  <div className="setting" id="handy-status">
                    <div>
                      <h3>
                        {intl.formatMessage({
                          id: "config.ui.handy_connection.status.heading",
                        })}
                      </h3>

                      <div className="value">
                        <FormattedMessage
                          id={connectionStateLabel(interactiveState)}
                        />
                        {interactiveError && <span>: {interactiveError}</span>}
                      </div>
                    </div>
                    <div>
                      {!interactiveInitialised && (
                        <Button
                          disabled={
                            interactiveState === ConnectionState.Connecting ||
                            interactiveState === ConnectionState.Syncing
                          }
                          onClick={() => initialiseInteractive()}
                        >
                          {intl.formatMessage({
                            id: "config.ui.handy_connection.connect",
                          })}
                        </Button>
                      )}
                    </div>
                  </div>
                  <div className="setting" id="handy-server-offset">
                    <div>
                      <h3>
                        {intl.formatMessage({
                          id: "config.ui.handy_connection.server_offset.heading",
                        })}
                      </h3>

                      <div className="value">
                        {interactiveServerOffset.toFixed()}ms
                      </div>
                    </div>
                    <div>
                      {interactiveInitialised && (
                        <Button
                          disabled={
                            !interactiveInitialised ||
                            interactiveState === ConnectionState.Syncing
                          }
                          onClick={() => interactiveSync()}
                        >
                          {intl.formatMessage({
                            id: "config.ui.handy_connection.sync",
                          })}
                        </Button>
                      )}
                    </div>
                  </div>
                </>
              )}

              <NumberSetting
                headingID="config.ui.funscript_offset.heading"
                subHeadingID="config.ui.funscript_offset.description"
                value={iface.funscriptOffset ?? undefined}
                onChange={(v) => saveInterface({ funscriptOffset: v })}
              />

              <BooleanSetting
                id="use-stash-hosted-funscript"
                headingID="config.ui.use_stash_hosted_funscript.heading"
                subHeadingID="config.ui.use_stash_hosted_funscript.description"
                checked={iface.useStashHostedFunscript ?? false}
                onChange={(v) => saveInterface({ useStashHostedFunscript: v })}
              />
            </SettingSection>
          </>
        )}

        <PersonalListSettings />
      </PersonalSettingsProvider>
    );
  }
);
