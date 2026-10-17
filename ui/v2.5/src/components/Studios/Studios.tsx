import React from "react";
import { Route, Switch } from "react-router-dom";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import Studio from "./StudioDetails/Studio";
import StudioCreate from "./StudioDetails/StudioCreate";
import { FilteredStudioList } from "./StudioList";
import { View } from "../List/views";
import { usePermissions } from "src/hooks/Permissions";
import * as GQL from "src/core/generated-graphql";
import { PermissionDenied } from "../PermissionDenied";

const Studios: React.FC = () => {
  return <FilteredStudioList view={View.Studios} />;
};

const StudioRoutes: React.FC = () => {
  const { can } = usePermissions();
  const titleProps = useTitleProps({ id: "studios" });
  return (
    <>
      <Helmet {...titleProps} />
      <Switch>
        <Route exact path="/studios" component={Studios} />
        <Route
          exact
          path="/studios/new"
          component={
            can(GQL.Permission.StudiosEdit) ? StudioCreate : PermissionDenied
          }
        />
        <Route path="/studios/:id/:tab?" component={Studio} />
      </Switch>
    </>
  );
};

export default StudioRoutes;
