import React, {
  useEffect,
  useRef,
  useState,
  useCallback,
  useMemo,
} from "react";
import {
  defineMessages,
  FormattedMessage,
  MessageDescriptor,
  useIntl,
} from "react-intl";
import { Nav, Navbar, Button, Dropdown, Modal } from "react-bootstrap";
import { IconDefinition } from "@fortawesome/fontawesome-svg-core";
import { LinkContainer } from "react-router-bootstrap";
import { Link, NavLink, useLocation, useHistory } from "react-router-dom";
import { HashLink } from "react-router-hash-link";
import Mousetrap from "mousetrap";

import SessionUtils from "src/utils/session";
import { Icon } from "src/components/Shared/Icon";
import { useConfigurationContext } from "src/hooks/Config";
import { ManualStateContext } from "./Help/context";
import { SettingsButton } from "./SettingsButton";
import {
  faArrowRightArrowLeft,
  faBars,
  faChartColumn,
  faCircleUser,
  faFilm,
  faHeart,
  faImage,
  faImages,
  faKey,
  faMapMarkerAlt,
  faPlayCircle,
  faQuestionCircle,
  faSignOutAlt,
  faTag,
  faUser,
  faUsers,
  faVideo,
} from "@fortawesome/free-solid-svg-icons";
import { baseURL } from "src/core/createClient";
import { PatchComponent } from "src/patch";
import { roleMessageID, usePermissions } from "src/hooks/Permissions";
import { ChangePasswordDialog } from "./Dialogs/ChangePasswordDialog";

interface IMenuItem {
  name: string;
  message: MessageDescriptor;
  href: string;
  icon: IconDefinition;
  hotkey: string;
  userCreatable?: boolean;
}

interface IAccountAction {
  messageID: string;
  icon: IconDefinition;
  onClick?: () => void;
  // a page of the UI
  to?: string;
  // a page of the server
  href?: string;
}
const messages = defineMessages({
  scenes: {
    id: "scenes",
    defaultMessage: "Scenes",
  },
  images: {
    id: "images",
    defaultMessage: "Images",
  },
  groups: {
    id: "groups",
    defaultMessage: "Groups",
  },
  markers: {
    id: "markers",
    defaultMessage: "Markers",
  },
  performers: {
    id: "performers",
    defaultMessage: "Performers",
  },
  studios: {
    id: "studios",
    defaultMessage: "Studios",
  },
  tags: {
    id: "tags",
    defaultMessage: "Tags",
  },
  galleries: {
    id: "galleries",
    defaultMessage: "Galleries",
  },
  sceneTagger: {
    id: "sceneTagger",
    defaultMessage: "Scene Tagger",
  },
  donate: {
    id: "donate",
    defaultMessage: "Donate",
  },
  statistics: {
    id: "statistics",
    defaultMessage: "Statistics",
  },
});

const allMenuItems: IMenuItem[] = [
  {
    name: "scenes",
    message: messages.scenes,
    href: "/scenes",
    icon: faPlayCircle,
    hotkey: "g s",
    userCreatable: true,
  },
  {
    name: "images",
    message: messages.images,
    href: "/images",
    icon: faImage,
    hotkey: "g i",
  },
  {
    name: "groups",
    message: messages.groups,
    href: "/groups",
    icon: faFilm,
    hotkey: "g v",
    userCreatable: true,
  },
  {
    name: "markers",
    message: messages.markers,
    href: "/scenes/markers",
    icon: faMapMarkerAlt,
    hotkey: "g k",
  },
  {
    name: "galleries",
    message: messages.galleries,
    href: "/galleries",
    icon: faImages,
    hotkey: "g l",
    userCreatable: true,
  },
  {
    name: "performers",
    message: messages.performers,
    href: "/performers",
    icon: faUser,
    hotkey: "g p",
    userCreatable: true,
  },
  {
    name: "studios",
    message: messages.studios,
    href: "/studios",
    icon: faVideo,
    hotkey: "g u",
    userCreatable: true,
  },
  {
    name: "tags",
    message: messages.tags,
    href: "/tags",
    icon: faTag,
    hotkey: "g t",
    userCreatable: true,
  },
];

const newPathsList = allMenuItems
  .filter((item) => item.userCreatable)
  .map((item) => item.href);

const MainNavbarMenuItems = PatchComponent(
  "MainNavBar.MenuItems",
  (props: React.PropsWithChildren<unknown>) => {
    return <Nav>{props.children}</Nav>;
  }
);

const MainNavbarUtilityItems = PatchComponent(
  "MainNavBar.UtilityItems",
  (props: React.PropsWithChildren<unknown>) => {
    return <>{props.children}</>;
  }
);

export const MainNavbar: React.FC = () => {
  const history = useHistory();
  const location = useLocation();
  const { configuration } = useConfigurationContext();
  const { openManual } = React.useContext(ManualStateContext);
  const { user, role, isAdmin, canModify } = usePermissions();

  // the drawer replaces the collapsed menu on phones and tablets
  const [showDrawer, setShowDrawer] = useState(false);
  const [showChangePassword, setShowChangePassword] = useState(false);

  // Show all menu items by default, unless config says otherwise
  const menuItems = useMemo(() => {
    let cfgMenuItems = configuration?.interface.menuItems;
    if (!cfgMenuItems) {
      return allMenuItems;
    }

    // translate old movies menu item to groups
    cfgMenuItems = cfgMenuItems.map((item) => {
      if (item === "movies") {
        return "groups";
      }
      return item;
    });

    return allMenuItems.filter((menuItem) =>
      cfgMenuItems!.includes(menuItem.name)
    );
  }, [configuration]);

  // react-bootstrap typing bug
  const navbarRef = useRef<HTMLElement | null>(null);
  const intl = useIntl();

  const closeDrawer = useCallback(() => setShowDrawer(false), []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: the drawer closes after any navigation
  useEffect(() => {
    setShowDrawer(false);
  }, [location.pathname, location.search]);

  const goto = useCallback(
    (page: string) => {
      history.push(page);
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur();
      }
    },
    [history]
  );

  const pathname = location.pathname.replace(/\/$/, "");
  let newPath =
    canModify && newPathsList.includes(pathname) ? `${pathname}/new` : null;
  if (newPath !== null) {
    const queryParam = new URLSearchParams(location.search).get("q");
    if (queryParam) {
      newPath += "?q=" + encodeURIComponent(queryParam);
    }
  }

  // set up hotkeys
  useEffect(() => {
    Mousetrap.bind("?", () => openManual());
    if (isAdmin) {
      Mousetrap.bind("g z", () => goto("/settings"));
    }

    menuItems.forEach((item) => {
      Mousetrap.bind(item.hotkey, () => goto(item.href));
    });

    if (newPath) {
      Mousetrap.bind("n", () => history.push(String(newPath)));
    }

    return () => {
      Mousetrap.unbind("?");
      Mousetrap.unbind("g z");
      menuItems.forEach((item) => {
        Mousetrap.unbind(item.hotkey);
      });

      if (newPath) {
        Mousetrap.unbind("n");
      }
    };
  });

  // scrolls to a section, clear of the navbar when it is at the top
  const scrollBelowNavbar = useCallback((el: HTMLElement) => {
    const navbar = navbarRef.current?.getBoundingClientRect();
    const offset = navbar && navbar.top <= 0 ? navbar.bottom : 0;
    window.scrollTo({
      top: el.getBoundingClientRect().top + window.scrollY - offset,
    });
  }, []);

  // without users there is no login, so no account
  const hasAccount = !!user || SessionUtils.isLoggedIn();
  const roleName = intl.formatMessage({ id: roleMessageID(role) });
  // the sign-in page in switch mode, which returns here on cancel
  const switchAccountURL = `${baseURL}login?${new URLSearchParams({
    switch: "1",
    returnURL: history.createHref({
      pathname: location.pathname,
      search: location.search,
    }),
  }).toString()}`;

  function onChangePassword() {
    closeDrawer();
    setShowChangePassword(true);
  }

  const userActions: IAccountAction[] = [];
  if (user) {
    userActions.push({
      messageID: "permissions.change_password",
      icon: faKey,
      onClick: onChangePassword,
    });
    if (isAdmin) {
      userActions.push({
        messageID: "permissions.manage_users",
        icon: faUsers,
        to: "/settings?tab=security#users",
      });
    }
  }

  const sessionActions: IAccountAction[] = [
    {
      messageID: "permissions.switch_account",
      icon: faArrowRightArrowLeft,
      href: switchAccountURL,
    },
    {
      messageID: "actions.logout",
      icon: faSignOutAlt,
      href: `${baseURL}logout`,
    },
  ];

  function renderMenuAction(a: IAccountAction) {
    const content = (
      <>
        <Icon icon={a.icon} />
        <FormattedMessage id={a.messageID} />
      </>
    );

    if (a.onClick) {
      return (
        <Dropdown.Item key={a.messageID} as="button" onClick={a.onClick}>
          {content}
        </Dropdown.Item>
      );
    }

    if (a.to) {
      return (
        <Dropdown.Item
          key={a.messageID}
          as={HashLink}
          to={a.to}
          scroll={scrollBelowNavbar}
        >
          {content}
        </Dropdown.Item>
      );
    }

    return (
      <Dropdown.Item key={a.messageID} href={a.href}>
        {content}
      </Dropdown.Item>
    );
  }

  function renderDrawerAction(a: IAccountAction) {
    const content = (
      <>
        <Icon icon={a.icon} />
        <FormattedMessage id={a.messageID} />
      </>
    );

    if (a.onClick) {
      return (
        <button
          key={a.messageID}
          type="button"
          className="nav-drawer-item"
          onClick={a.onClick}
        >
          {content}
        </button>
      );
    }

    if (a.to) {
      return (
        <HashLink
          key={a.messageID}
          className="nav-drawer-item"
          to={a.to}
          scroll={scrollBelowNavbar}
          onClick={closeDrawer}
        >
          {content}
        </HashLink>
      );
    }

    return (
      <a key={a.messageID} className="nav-drawer-item" href={a.href}>
        {content}
      </a>
    );
  }

  function renderAccountHeader() {
    if (!user) return;

    return (
      <>
        <Icon icon={faCircleUser} className="account-menu-avatar" />
        <div className="account-menu-user">
          <div className="account-menu-name">{user.name}</div>
          <div className="account-menu-role">{roleName}</div>
        </div>
      </>
    );
  }

  function maybeRenderAccountMenu() {
    if (!hasAccount) return;

    return (
      <Dropdown className="nav-utility account-menu">
        <Dropdown.Toggle
          id="account-menu"
          className="minimal d-flex align-items-center h-100"
          title={
            user
              ? intl.formatMessage(
                  { id: "permissions.logged_in_as" },
                  { name: user.name, role: roleName }
                )
              : intl.formatMessage({ id: "permissions.account" })
          }
        >
          <Icon icon={faCircleUser} />
        </Dropdown.Toggle>
        <Dropdown.Menu alignRight>
          {user && (
            <>
              <Dropdown.Header className="account-menu-header">
                {renderAccountHeader()}
              </Dropdown.Header>
              {userActions.map(renderMenuAction)}
              <Dropdown.Divider />
            </>
          )}
          {sessionActions.map(renderMenuAction)}
        </Dropdown.Menu>
      </Dropdown>
    );
  }

  function maybeRenderDrawerAccount() {
    if (!hasAccount) return;

    return (
      <div className="nav-drawer-account">
        {user && (
          <div className="account-menu-header">{renderAccountHeader()}</div>
        )}
        {userActions.map(renderDrawerAction)}
        {sessionActions.map(renderDrawerAction)}
      </div>
    );
  }

  function renderUtilityButtons(withAccount: boolean) {
    return (
      <>
        <Nav.Link
          className="nav-utility"
          href="https://opencollective.com/stashapp"
          target="_blank"
          onClick={closeDrawer}
        >
          <Button
            className="minimal donate"
            title={intl.formatMessage({ id: "donate" })}
          >
            <Icon icon={faHeart} />
            <span className="d-none d-sm-inline">
              {intl.formatMessage(messages.donate)}
            </span>
          </Button>
        </Nav.Link>
        <NavLink
          className="nav-utility"
          exact
          to="/stats"
          onClick={closeDrawer}
        >
          <Button
            className="minimal d-flex align-items-center h-100"
            title={intl.formatMessage({ id: "statistics" })}
          >
            <Icon icon={faChartColumn} />
          </Button>
        </NavLink>
        {isAdmin && (
          <NavLink
            className="nav-utility"
            exact
            to="/settings"
            onClick={closeDrawer}
          >
            <SettingsButton />
          </NavLink>
        )}
        <Button
          className="nav-utility minimal"
          onClick={() => {
            closeDrawer();
            openManual();
          }}
          title={intl.formatMessage({ id: "help" })}
        >
          <Icon icon={faQuestionCircle} />
        </Button>
        {withAccount && maybeRenderAccountMenu()}
      </>
    );
  }

  function renderDrawer() {
    return (
      <Modal
        show={showDrawer}
        onHide={closeDrawer}
        className="nav-drawer"
        backdropClassName="nav-drawer-backdrop"
        aria-labelledby="nav-drawer-title"
      >
        <Modal.Header
          closeButton
          closeLabel={intl.formatMessage({ id: "actions.close" })}
        >
          <Modal.Title as="div" id="nav-drawer-title">
            Stash
          </Modal.Title>
        </Modal.Header>
        <Modal.Body>
          {maybeRenderDrawerAccount()}
          <MainNavbarMenuItems>
            {menuItems.map(({ href, icon, message }) => (
              <NavLink
                key={href}
                exact
                to={href}
                className="nav-drawer-item"
                activeClassName="active"
                onClick={closeDrawer}
              >
                <Icon icon={icon} />
                <span>{intl.formatMessage(message)}</span>
              </NavLink>
            ))}
          </MainNavbarMenuItems>
          <div className="nav-drawer-utility">
            <MainNavbarUtilityItems>
              {renderUtilityButtons(false)}
            </MainNavbarUtilityItems>
          </div>
        </Modal.Body>
      </Modal>
    );
  }

  return (
    <>
      <Navbar
        fixed="top"
        variant="dark"
        bg="dark"
        className="top-nav"
        expand="xl"
        ref={navbarRef}
      >
        <Navbar.Collapse className="bg-dark order-sm-1">
          <MainNavbarMenuItems>
            {menuItems.map(({ href, icon, message }) => (
              <Nav.Link
                eventKey={href}
                as="div"
                key={href}
                className="col-4 col-sm-3 col-md-2 col-lg-auto"
              >
                <LinkContainer activeClassName="active" exact to={href}>
                  <Button className="minimal p-4 p-xl-2 d-flex d-xl-inline-block flex-column justify-content-between align-items-center">
                    <Icon
                      {...{ icon }}
                      className="nav-menu-icon d-block d-xl-inline mb-2 mb-xl-0"
                    />
                    <span>{intl.formatMessage(message)}</span>
                  </Button>
                </LinkContainer>
              </Nav.Link>
            ))}
          </MainNavbarMenuItems>
        </Navbar.Collapse>

        <Navbar.Brand as="div">
          <Link to="/">
            <Button className="minimal brand-link d-inline-block">Stash</Button>
          </Link>
        </Navbar.Brand>

        <Nav className="navbar-buttons flex-row ml-auto order-xl-2">
          {!!newPath && (
            <div className="mr-2">
              <Link to={newPath}>
                <Button variant="primary" data-action="new">
                  <FormattedMessage id="new" defaultMessage="New" />
                </Button>
              </Link>
            </div>
          )}
          <MainNavbarUtilityItems>
            {renderUtilityButtons(true)}
          </MainNavbarUtilityItems>
          <button
            type="button"
            className="navbar-toggler nav-menu-toggle ml-sm-2"
            aria-label="Toggle navigation"
            aria-haspopup="dialog"
            aria-expanded={showDrawer}
            onClick={() => setShowDrawer(true)}
          >
            <Icon icon={faBars} />
          </button>
        </Nav>
      </Navbar>
      {renderDrawer()}
      {showChangePassword && user && (
        <ChangePasswordDialog
          username={user.name}
          hasPassword={user.hasPassword}
          onClose={() => setShowChangePassword(false)}
        />
      )}
    </>
  );
};
