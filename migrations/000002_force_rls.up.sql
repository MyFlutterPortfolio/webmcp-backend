-- Defense in depth: table owners must obey tenant isolation policies too.
-- Without FORCE, a mistakenly over-privileged application role can bypass RLS.
DO $$
DECLARE table_name text;
BEGIN
    FOREACH table_name IN ARRAY ARRAY['users','businesses','products','customers','orders','metrics','goals','goal_constraints','scenarios','proposals','proposal_versions','approvals','commit_operations','committed_decisions','audit_events'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', table_name);
    END LOOP;
END $$;
