var React = require("react");
var ReactDOM = require("react-dom");
var Redux = require("redux");
var ReactRedux = require("react-redux");
var thunkMiddleware = require("redux-thunk");
var fetch = require("isomorphic-fetch");

var api = "http://localhost:8080";

// Action types

var SHOW_EXPLORER = "SHOW_EXPLORER";
var HIDE_EXPLORER = "HIDE_EXPLORER";
var TOGGLE_EXPLORER = "TOGGLE_EXPLORER";

var TAG_FETCHING_START = "FETCING_START";
var TAG_FETCHING_ERROR = "FETCHING_ERROR";
var TAG_FETCHING_SUCCESS = "FETCHING_SUCCESS";

// Action creators

var showExplorer = function() {
  return { type: SHOW_EXPLORER };
};
var hideExplorer = function() {
  return { type: HIDE_EXPLORER };
};
var toggleExplorer = function() {
  return { type: TOGGLE_EXPLORER };
};

var tagFetchingStart = function() {
  return { type: TAG_FETCHING_START };
};
var tagFetchingError = function(err) {
  return {
    type: TAG_FETCHING_ERROR,
    err: err,
  };
};
var tagFetchingSuccess = function(tags) {
  console.log("got tags"+JSON.stringify(tags, null, "  "));
  return {
    type: TAG_FETCHING_SUCCESS,
    tags: tags,
  };
};
var fetchTags = function() {
  return function(dispatch) {
    dispatch(tagFetchingStart());

    return fetch(api+"/tags")
    .then(function(response) {
      return response.json();
    })
    .then(function(json) {
      if (!json || json.length === 0) {
        dispatch(showExplorer);
      }
      dispatch(tagFetchingSuccess(json));
    })
    .catch(function(err) {
      // hope this is actually and Error object like I promised (sic)
      dispatch(tagFetchingError(err));
    });
  };
};

// Reducers

var explorer = function(state, action) {
  if (state === undefined) {
    state = false;
  }
  switch (action.type) {
    case SHOW_EXPLORER:
      return true;
    case HIDE_EXPLORER:
      return false;
    case TOGGLE_EXPLORER:
      return !state;
    default:
      return state;
  }
};

var tagCloud = function(state, action) {
  if (!state) {
    state = {
      // null means haven't started yet.
      // true means there's a request in flight.
      // false means a request completed successfully.
      // object is an Error and means a request failed.
      loading: null,
      tags: [],
    };
  }
  switch (action.type) {
  case TAG_FETCHING_START:
    return Object.assign({}, state, {
      loading: true,
    });
  case TAG_FETCHING_ERROR:
    return Object.assign({}, state, {
      loading: action.err,
    });
  case TAG_FETCHING_SUCCESS:
    return Object.assign({}, state, {
      loading: false,
      tags: action.tags,
    });
  default:
    return state;
  }
};

var trixelTagger = Redux.combineReducers({
  explorer: explorer,
  tagCloud: tagCloud,
});

// Components

// Selector displays a tag cloud for selecting tags. Trixels tagged with all
// selected tags are displayed.
var Selector = React.createClass({
  propTypes: {
    tags: React.PropTypes.array.isRequired,
    loading: React.PropTypes.any.isRequired,
    retry: React.PropTypes.func.isRequired,
  },
  render: function() {
    switch (this.props.loading) {
    case null:
    case true:
      return <div className="tag-cloud">
        <img className="tag-cloud-spinner" src="spinner.svg" />
      </div>;
    case false:
      var tags = this.props.tags;

      if (!tags || tags.length === 0) {
        return <div>No tags</div>;
      }

      return (
        <div className="tag-cloud">
          {this.props.tags.map(function(tag, i) {
            return <span className="tag-cloud-tag" key={i}>{tag}</span>;
          })}
        </div>
      );
    default:
      return (
        <div className="tag-cloud">
          <div className="tag-cloud-error">
            Error loading tags: {this.props.loading.message}
          </div>
          <button onClick={this.props.retry}>Retry</button>
        </div>
      );
    }
  },
});

var explorerButtonPropTypes = {
  explorerShowing: React.PropTypes.bool.isRequired,
  onToggleClick: React.PropTypes.func.isRequired,
};

var ExplorerButton = React.createClass({
  propTypes: explorerButtonPropTypes,
  render: function() {
    return (
      <button className="explorer-button" onClick={this.props.onToggleClick}>
        {(this.props.explorerShowing) ? "-" : "+"}
      </button>
    );
  },
});

// Explorer is used to find trixels you haven't yet tagged.
var Explorer = React.createClass({
  propTypes: explorerButtonPropTypes,
  render: function() {
    if (!this.props.explorerShowing) {
      return <ExplorerButton explorerShowing={this.props.explorerShowing}
        onToggleClick={this.props.onToggleClick} />;
    }
    return (
      <div className="explore">
        <ExplorerButton explorerShowing={this.props.explorerShowing}
          onToggleClick={this.props.onToggleClick} />
      </div>
    );
  },
});

// Trixel displays a trixel.
var Trixel = React.createClass({
  propTypes: {
    tags: React.PropTypes.array,
    id: React.PropTypes.string.isRequired,
  },
  render: function() {
    var tags = this.props.tags;
    var children;
    if (tags && tags.length) {
      children = [];
      for (var i = 0; i < tags.length; i++) {
        if (i !== 0) {
          children.push(<span key={"sep-" + i}>, </span>);
        }
        children.push(
          <span className="trixel-tag" key={"tag-" + i}>
            {tags[i]}
          </span>
        );
      }
    }
    return (
      <div className="trixel">
        <img className="trixel-img"
          src={"http://cdn.trixel.io/trixels/"+this.props.id+".svg"} />
        <div className="trixel-tags">{children}</div>
      </div>
    );
  },
});

var TrixelTagger = React.createClass({
  render: function() {
    var dispatch = this.props.dispatch; // from react-redux
    return (
      <div>
        <h1>Trixel Tagger</h1>
        <Explorer
          explorerShowing={this.props.explorer}
          onToggleClick={function() { dispatch(toggleExplorer()); }}
        />
        <div>
          <Selector {...this.props.tagCloud}
            retry={function() { dispatch(fetchTags()); }} />
          <div>
            <Trixel id="1999" tags={["ooo", "arr"]} />
            <Trixel id="1998" tags={["ooo", "arr"]} />
            <Trixel id="1000" tags={["ooo", "arr"]} />
          </div>
        </div>
      </div>
    );
  },
});

var select = function(state) {
  return state;
};

var Top = ReactRedux.connect(select)(TrixelTagger);

var middleware = Redux.applyMiddleware(thunkMiddleware);
var createStore = middleware(Redux.createStore);
var store = createStore(trixelTagger);

ReactDOM.render(
  <ReactRedux.Provider store={store}>
    <Top />
  </ReactRedux.Provider>,
  document.getElementById("mountpoint")
);

// store.dispatch(fetchTags());
store.dispatch(tagFetchingError(new Error("testing")));
