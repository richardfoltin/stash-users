import React from "react";
import { Route, Switch } from "react-router-dom";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import Group from "./GroupDetails/Group";
import GroupCreate from "./GroupDetails/GroupCreate";
import { FilteredGroupList } from "./GroupList";
import { View } from "../List/views";
import { usePermissions } from "src/hooks/Permissions";
import * as GQL from "src/core/generated-graphql";
import { PermissionDenied } from "../PermissionDenied";

const Groups: React.FC = () => {
  return <FilteredGroupList view={View.Groups} />;
};

const GroupRoutes: React.FC = () => {
  const { can } = usePermissions();
  const titleProps = useTitleProps({ id: "groups" });
  return (
    <>
      <Helmet {...titleProps} />
      <Switch>
        <Route exact path="/groups" component={Groups} />
        <Route
          exact
          path="/groups/new"
          component={
            can(GQL.Permission.GroupsEdit) ? GroupCreate : PermissionDenied
          }
        />
        <Route path="/groups/:id/:tab?" component={Group} />
      </Switch>
    </>
  );
};

export default GroupRoutes;
