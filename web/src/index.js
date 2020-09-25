import React from 'react'
import ReactDOM from 'react-dom'
import { BrowserRouter, Route, Switch } from 'react-router-dom'

import { BaseVideo, CreateVideo, EditVideo } from './api/video.js'

import { BaseProblem } from './api/problem.js'

import 'foundation-sites/dist/css/foundation.css'
import './index.css'

const NotFound = () => (
  <div>
    <h3>404 page not found</h3>
  </div>
)

/*
 * TODO:
 * - react import files
 * - video.js file
 *   - video display (id as input)
 *   - video create form
 *   - video edit form
 */

var conf = require('./conf')
const ApiUrl = conf.api_host + ':' + conf.api_port + '/api/v1'

class AdminVideosView extends React.Component {
  render() {
    return (
      <div>
        <CreateVideo url={ApiUrl} />
        <hr />
        <BaseVideo
          id={1}
          url={ApiUrl}
          public_video_dir={conf.public_video_dir}
        />
        <hr />
        <BaseVideo
          id={2}
          url={ApiUrl}
          public_video_dir={conf.public_video_dir}
        />
        <hr />
        <EditVideo id={1} url={ApiUrl} />
      </div>
    )
  }
}

class ProblemView extends React.Component {
  render() {
    return (
      <div>
        <BaseProblem url={ApiUrl} />
      </div>
    )
  }
}

const Main = () => (
  <main>
    <Switch>
      <Route path="/admin/videos" component={AdminVideosView} />
      <Route path="/problem" component={ProblemView} />
      <Route path="*" component={NotFound} />
    </Switch>
  </main>
)

const App = () => (
  <div>
    <div className="top-bar">
      <div className="top-bar-left">
        <ul className="menu">
          <li className="menu-text">The Math Game</li>
        </ul>
      </div>

      <div className="top-bar-right">
        <ul className="menu">
          <li>
            <a href="/">log in</a>
          </li>
          <li>
            <a href="/">2</a>
          </li>
          <li>
            <a href="/problem">3</a>
          </li>
        </ul>
      </div>
    </div>

    <div className="grid-container full">
      <div className="grid-x grid-margin-x align-center">
        <div className="cell small-11 medium-8 large-7">
          <Main />
        </div>
      </div>
    </div>
  </div>
)

ReactDOM.render(
  <BrowserRouter>
    <App />
  </BrowserRouter>,
  document.getElementById('react')
)
