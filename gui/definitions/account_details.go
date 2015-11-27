
package definitions

func init(){
  add(`AccountDetails`, &defAccountDetails{})
}

type defAccountDetails struct{}

func (*defAccountDetails) String() string {
	return `
<interface>
  <object class="GtkDialog" id="AccountDetails">
    <property name="title" translatable="yes">Account Details</property>
    <signal name="close" handler="on_cancel_signal" />
    <child internal-child="vbox">
      <object class="GtkBox" id="Vbox">
        <property name="margin">10</property>
        <child>
          <object class="GtkGrid" id="grid">
            <property name="margin-bottom">10</property>
            <property name="row-spacing">12</property>
            <property name="column-spacing">6</property>
            <child>
              <object class="GtkLabel" id="AccountMessageLabel">
                <property name="label" translatable="yes">Your account&#xA;(example: kim42@dukgo.com)</property>
                <property name="justify">GTK_JUSTIFY_RIGHT</property>
              </object>
              <packing>
                <property name="left-attach">0</property>
                <property name="top-attach">0</property>
              </packing>
            </child>
            <child>
              <object class="GtkEntry" id="account">
                <signal name="activate" handler="on_save_signal" />
              </object>
              <packing>
                <property name="left-attach">1</property>
                <property name="top-attach">0</property>
              </packing>
            </child>
            <child>
              <object class="GtkLabel" id="PasswordLabel">
                <property name="label" translatable="yes">Password</property>
                <property name="halign">GTK_ALIGN_END</property>
              </object>
              <packing>
                <property name="left-attach">0</property>
                <property name="top-attach">1</property>
              </packing>
            </child>
            <child>
              <object class="GtkEntry" id="password">
                <property name="visibility">false</property>
                <signal name="activate" handler="on_save_signal" />
              </object>
              <packing>
                <property name="left-attach">1</property>
                <property name="top-attach">1</property>
              </packing>
            </child>
          </object>
        </child>
      </object>
      <object class="GtkHBox" id="Hbox">
        <child>
          <object class="GtkButton" id="cancel">
            <property name="label" translatable="yes">Cancel</property>
            <signal name="clicked" handler="on_cancel_signal"/>
          </object>
        </child>
        <child>
          <object class="GtkButton" id="save">
            <property name="label" translatable="yes">Save</property>
            <property name="can-default">true</property>
            <signal name="clicked" handler="on_save_signal"/>
          </object>
        </child>
      </object>
    </child>
  </object>
</interface>

`
}
