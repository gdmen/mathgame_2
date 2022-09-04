
Features to add:
* Difficulty scaling based on % work time
* configuration of target work % time
* problem generation algorithm work
  * word problems
  * generate better problems
  * should be able to take an expression as input and output a difficulty (each time we update the problem generation algorithm, we need to recalc the difficulty of existing problems)
* choose videos for this user
  * INSERT INTO userHasVideos (user_id, video_id) VALUES (1, 2);

//

- Add a cron that will simplify event logs (e.g. merge adjacent watching_video events)

- Handler test data should be in a separate file & should be auto-updated by the tests

- Hardcoded API token for writes

- Add failure case unit tests

- Move server/common/middleware and log to api
- Move server/common/common to lib


// the number(+1) of problems Trevor's done so far
- SELECT COUNT(DISTINCT(`value`)) FROM `events` WHERE `user_id` = 7 AND `event_type` = "displayed_problem"
