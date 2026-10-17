import React from "react";
import { Route, Switch } from "react-router-dom";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import Tag from "./TagDetails/Tag";
import TagCreate from "./TagDetails/TagCreate";
import { FilteredTagList } from "./TagList";
import { usePermissions } from "src/hooks/Permissions";
import { PermissionDenied } from "../PermissionDenied";

const Tags: React.FC = () => {
  return <FilteredTagList />;
};

const TagRoutes: React.FC = () => {
  const { canModify } = usePermissions();
  const titleProps = useTitleProps({ id: "tags" });
  return (
    <>
      <Helmet {...titleProps} />
      <Switch>
        <Route exact path="/tags" component={Tags} />
        <Route
          exact
          path="/tags/new"
          component={canModify ? TagCreate : PermissionDenied}
        />
        <Route path="/tags/:id/:tab?" component={Tag} />
      </Switch>
    </>
  );
};

export default TagRoutes;
