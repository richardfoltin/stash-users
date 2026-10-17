import React from "react";
import { Route, Switch } from "react-router-dom";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import Performer from "./PerformerDetails/Performer";
import PerformerCreate from "./PerformerDetails/PerformerCreate";
import { FilteredPerformerList } from "./PerformerList";
import { View } from "../List/views";
import { usePermissions } from "src/hooks/Permissions";
import * as GQL from "src/core/generated-graphql";
import { PermissionDenied } from "../PermissionDenied";

const Performers: React.FC = () => {
  return <FilteredPerformerList view={View.Performers} />;
};

const PerformerRoutes: React.FC = () => {
  const { can } = usePermissions();
  const titleProps = useTitleProps({ id: "performers" });
  return (
    <>
      <Helmet {...titleProps} />
      <Switch>
        <Route exact path="/performers" component={Performers} />
        <Route
          path="/performers/new"
          component={
            can(GQL.Permission.PerformersEdit)
              ? PerformerCreate
              : PermissionDenied
          }
        />
        <Route path="/performers/:id/:tab?" component={Performer} />
      </Switch>
    </>
  );
};

export default PerformerRoutes;
